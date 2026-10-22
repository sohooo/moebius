import datetime as dt
import json
from pathlib import Path
import tempfile
import unittest
from monitor import evaluate, go_findings, read_reports, trivy_findings

NOW = dt.datetime(2026, 10, 22, tzinfo=dt.timezone.utc)


class MonitorTests(unittest.TestCase):
    def test_first_detection_repeat_and_weekly_reminder(self):
        state, alert, _ = evaluate(None, {'source': ['finding']}, NOW)
        self.assertTrue(alert)
        again, alert, _ = evaluate(state, {'source': ['finding']}, NOW + dt.timedelta(days=1))
        self.assertFalse(alert)
        self.assertEqual(state['last_alert'], again['last_alert'])
        _, alert, reason = evaluate(again, {'source': ['finding']}, NOW + dt.timedelta(days=7))
        self.assertTrue(alert)
        self.assertEqual(reason, 'weekly reminder')

    def test_new_findings_and_fix_changes_alert_immediately(self):
        state, _, _ = evaluate(None, {'source': ['old']}, NOW)
        _, alert, _ = evaluate(state, {'source': ['old', 'new']}, NOW)
        self.assertTrue(alert)
        _, alert, _ = evaluate(state, {'source': ['new fix version']}, NOW)
        self.assertTrue(alert)

    def test_resolution_and_recurrence(self):
        state, _, _ = evaluate(None, {'source': ['finding']}, NOW)
        state, alert, _ = evaluate(state, {'source': []}, NOW)
        self.assertFalse(alert)
        _, alert, _ = evaluate(state, {'source': ['finding']}, NOW)
        self.assertTrue(alert)

    def test_skipped_weekly_scan_does_not_clear_findings(self):
        state, _, _ = evaluate(None, {'refreshed-image': ['finding']}, NOW)
        state, alert, _ = evaluate(state, {'source': []}, NOW + dt.timedelta(days=1))
        self.assertFalse(alert)
        self.assertEqual(state['snapshots']['refreshed-image'], ['finding'])
        _, alert, _ = evaluate(state, {'source': []}, NOW + dt.timedelta(days=7))
        self.assertTrue(alert)

    def test_empty_history_and_missing_reports(self):
        _, alert, _ = evaluate(None, {'source': []}, NOW)
        self.assertFalse(alert)
        with tempfile.TemporaryDirectory() as folder:
            with self.assertRaises(ValueError):
                read_reports(Path(folder))
        with self.assertRaises(ValueError):
            trivy_findings({'Results': []})
        with self.assertRaises(ValueError):
            evaluate({'schema': 99}, {}, NOW)

    def test_trivy_order_and_duplicate_findings_are_stable(self):
        high = {'VulnerabilityID': 'CVE-test', 'PkgName': 'stdlib',
                'InstalledVersion': '1', 'FixedVersion': '2', 'Severity': 'HIGH'}
        medium = {**high, 'Severity': 'MEDIUM'}
        report = {'Results': [{'Type': 'gobinary', 'Vulnerabilities': [high, medium, high]}]}
        self.assertEqual(len(trivy_findings(report)), 1)
        report['Results'][0]['Vulnerabilities'].reverse()
        self.assertEqual(len(trivy_findings(report)), 1)

    def test_go_reachability_and_invalid_input(self):
        messages = [{'config': {}},
                    {'finding': {'osv': 'module-only', 'trace': [{'module': 'x'}]}},
                    {'finding': {'osv': 'called', 'fixed_version': 'v2', 'trace': [
                        {'module': 'x', 'version': 'v1', 'function': 'Verify'}]}}]
        findings = go_findings('\n'.join(map(json.dumps, messages)))
        self.assertEqual([json.loads(f)['id'] for f in findings], ['called'])
        for bad in ('', '{"config": {}} invalid', '{"config":{}} {"error":"failed"}'):
            with self.assertRaises(ValueError):
                go_findings(bad)


if __name__ == '__main__':
    unittest.main()
