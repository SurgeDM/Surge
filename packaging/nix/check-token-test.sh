#!/usr/bin/env bash
set -euo pipefail

test_dir=$(mktemp -d)
trap 'chmod -R u+rwX "$test_dir"; rm -rf "$test_dir"' EXIT
checker=$(cd "$(dirname "$0")" && pwd)/check-token.sh
token="$test_dir/override token"
state="$test_dir/state token"
started="$test_dir/started"
ln -s "$token" "$state"

reject() {
  if bash "$checker" "$token" "$state" touch "$started" 2>"$test_dir/error"; then
    echo "Unexpected startup success: $1" >&2
    exit 1
  fi
  test ! -e "$started"
  test -s "$test_dir/error"
}

reject 'missing file'
test ! -e "$token"
mkdir "$token"
reject 'directory instead of file'
rmdir "$token"
: > "$token"
reject 'empty file'
printf ' \t\r\n' > "$token"
reject 'whitespace-only file'
printf 'test-secret\n' > "$token"
chmod 000 "$token"
# Root can read mode-000 files; normal users and Nix builders exercise this case.
if [ "$(id -u)" != 0 ]; then
  reject 'unreadable file'
fi
chmod 600 "$token"
rm "$state"
reject 'missing state link'
printf 'another-token\n' > "$state"
reject 'replaced state link'
rm "$state"
ln -s "$token" "$state"
SURGE_TOKEN=inherited-token bash "$checker" "$token" "$state" bash -c '
  test "${SURGE_TOKEN+x}" != x || exit 1
  touch "$1"
' _ "$started"
test -f "$started"
test "$(cat "$token")" = test-secret
printf 'Token startup checks passed\n'
