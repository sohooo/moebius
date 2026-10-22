"""Deduplicate scheduled vulnerability alerts using trusted workflow artifacts.

CI/release gates do not use this module. Infrastructure failures must prevent a
new baseline from being saved, so a failed scan cannot clear existing findings.
"""
import datetime as dt
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import zipfile

UTC = dt.timezone.utc
REQUIRED = {'published-image-amd64', 'published-image-arm64', 'source-security-report'}


def json_stream(text):
    decoder = json.JSONDecoder()
    text = text.lstrip()
    while text:
        item, end = decoder.raw_decode(text)
        yield item
        text = text[end:].lstrip()


def trivy_findings(report):
    results = report.get('Results', [])
    if not any(r.get('Type') == 'gobinary' for r in results):
        raise ValueError('Trivy did not detect a Go binary')
    return sorted({json.dumps({
        'id': v['VulnerabilityID'], 'package': v['PkgName'],
        'installed': v['InstalledVersion'], 'fixed': v.get('FixedVersion', ''),
        'severity': v['Severity'],
    }, sort_keys=True) for r in results for v in (r.get('Vulnerabilities') or [])
        if v['Severity'] in ('HIGH', 'CRITICAL')})


def go_findings(text):
    messages = list(json_stream(text))
    if not any('config' in m for m in messages):
        raise ValueError('Missing govulncheck configuration: incomplete scan')
    if any('error' in m for m in messages):
        raise ValueError('govulncheck reported an operational error')
    # Module/package-level findings have no function in the first trace frame.
    return sorted({json.dumps({
        'id': f['osv'], 'package': f['trace'][0].get('module', ''),
        'installed': f['trace'][0].get('version', ''),
        'fixed': f.get('fixed_version', ''), 'severity': 'REACHABLE',
    }, sort_keys=True) for m in messages if (f := m.get('finding'))
        and f.get('trace') and f['trace'][0].get('function')})


def read_reports(root):
    snapshots = {}
    for name in REQUIRED | {'refreshed-image'}:
        folder = root / name
        if not folder.exists():
            if name in REQUIRED:
                raise ValueError(f'Missing required report: {name}')
            continue
        if name == 'source-security-report':
            snapshots[name] = go_findings((folder / 'govulncheck.json').read_text())
        else:
            filename = 'refreshed.json' if name == 'refreshed-image' else 'image.json'
            snapshots[name] = trivy_findings(json.loads((folder / filename).read_text()))
    return snapshots


def evaluate(previous, scanned, now):
    if previous is not None and previous.get('schema') != 1:
        raise ValueError('Unsupported monitoring state schema')
    old = (previous or {}).get('snapshots', {})
    current = {**old, **scanned}  # Preserve the weekly result on daily-only runs.
    additions = {name: sorted(set(items) - set(old.get(name, [])))
                 for name, items in scanned.items()}
    changed = any(additions.values())
    outstanding = sum(len(items) for items in current.values())
    last = (previous or {}).get('last_alert')
    reminder = bool(outstanding and (not last or
        now - dt.datetime.fromisoformat(last) >= dt.timedelta(days=7)))
    notify = bool(outstanding and (changed or reminder))
    reason = 'new or changed findings' if changed else 'weekly reminder' if notify else 'unchanged findings' if outstanding else 'no outstanding findings'
    return {'schema': 1, 'snapshots': current,
            'last_alert': now.isoformat() if notify else last,
            'observed_at': now.isoformat()}, notify, reason


def gh_json(endpoint):
    return json.loads(subprocess.check_output(['gh', 'api', endpoint]))


def load_previous(repo, run_id):
    # Only trust scheduled/manual runs of this workflow on master, never PR artifacts.
    runs = gh_json(f'repos/{repo}/actions/workflows/security.yml/runs?status=completed&branch=master&per_page=100')['workflow_runs']
    for run in runs:
        if str(run['id']) == run_id or run['event'] not in ('schedule', 'workflow_dispatch'):
            continue
        artifacts = gh_json(f'repos/{repo}/actions/runs/{run["id"]}/artifacts')['artifacts']
        for artifact in artifacts:
            if artifact['name'] != 'security-monitor-state' or artifact['expired']:
                continue
            archive = subprocess.check_output(['gh', 'api', f'repos/{repo}/actions/artifacts/{artifact["id"]}/zip'])
            with zipfile.ZipFile(io.BytesIO(archive)) as z:
                return json.loads(z.read('state.json'))
    # First run or expired history: alert on every current finding, never suppress.
    return None


def main():
    root, output = Path(sys.argv[1]), Path(sys.argv[2])
    previous = load_previous(os.environ['GITHUB_REPOSITORY'], os.environ['GITHUB_RUN_ID'])
    scanned = read_reports(root)
    state, notify, reason = evaluate(previous, scanned, dt.datetime.now(UTC))
    output.mkdir(parents=True, exist_ok=True)
    (output / 'state.json').write_text(json.dumps(state, indent=2) + '\n')
    with open(os.environ['GITHUB_OUTPUT'], 'a') as f:
        f.write(f'notify={str(notify).lower()}\n')
    with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as f:
        f.write(f'## Security monitoring: {reason}\n\n')
        f.write('Unchanged findings remain open even when this workflow is green. CI and release gates remain strict.\n\n')
        for name, items in sorted(state['snapshots'].items()):
            f.write(f'### {name}\n\n')
            if name not in scanned:
                f.write('Retained from the previous weekly scan.\n\n')
            if not items:
                f.write('No actionable findings.\n\n')
            for item in items:
                v = json.loads(item)
                f.write(f'- `{v["id"]}` — `{v["package"]}` {v["installed"]}; {v["severity"]}; fix: {v["fixed"] or "not available"}\n')
            f.write('\n')
    print(reason)


if __name__ == '__main__':
    main()
