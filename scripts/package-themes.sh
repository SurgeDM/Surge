#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT}/.goreleaser-extra"

if ! command -v zip >/dev/null 2>&1; then
  echo "zip is required to package release assets" >&2
  exit 1
fi

mkdir -p "${OUT_DIR}"

echo "=== Packaging themes ==="

# Create a temporary directory to structure the zip file
TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

# We want the zip to contain a 'themes' folder
mkdir -p "${TMP_DIR}/themes"
cp "${ROOT}/themes"/*.toml "${TMP_DIR}/themes/"

(
  cd "${TMP_DIR}"
  zip -r -9 "${OUT_DIR}/themes.zip" themes -x "*.DS_Store"
)

echo "=== Themes packaged to ${OUT_DIR}/themes.zip ==="
