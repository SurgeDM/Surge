#!/usr/bin/env bash
set -euo pipefail

visual_repo_root="$(cd "$(dirname "$0")/.." && pwd)"
visual_test_dir="$(mktemp -d)"
trap 'rm -rf "$visual_test_dir"' EXIT

cd "$visual_repo_root"
go test -c -o "$visual_test_dir/surge-tui.test" ./internal/tui
SURGE_VISUAL_TEST=1 "$visual_test_dir/surge-tui.test" -test.run '^TestVisualTUI$' -test.count=1 "$@"
