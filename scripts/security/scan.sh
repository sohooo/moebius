#!/usr/bin/env bash
# Keep the complete report, then apply the release gate to the same scan.
set -euo pipefail
kind=${1:?usage: scan.sh image|rootfs TARGET REPORT_PREFIX [PLATFORM]}
target=${2:?missing target}
prefix=${3:?missing report prefix}
mkdir -p "$(dirname "$prefix")"
extra=()
if [[ $# -ge 4 ]]; then extra+=(--platform "$4"); fi
trivy "$kind" "${extra[@]}" --scanners vuln --format json --output "$prefix.json" "$target"
trivy convert --format sarif --output "$prefix.sarif" "$prefix.json"
# An empty/unsupported scan must never pass the release gate.
jq -e 'any(.Results[]?; .Type == "gobinary")' "$prefix.json" >/dev/null
trivy convert --format table --severity HIGH,CRITICAL --exit-code 1 "$prefix.json"
