#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT}/.goreleaser-extra"

if ! command -v zip >/dev/null 2>&1; then
  echo "zip is required to package release assets" >&2
  exit 1
fi

mkdir -p "${OUT_DIR}"
rm -f "${OUT_DIR}/extension-chrome.zip" \
  "${OUT_DIR}/extension-firefox.zip" \
  "${OUT_DIR}/fonts.zip"

(
  cd "${ROOT}/extension-chrome"
  zip -r -9 "${OUT_DIR}/extension-chrome.zip" . -x "*.DS_Store"
)

(
  cd "${ROOT}/extension-firefox"
  zip -r -9 "${OUT_DIR}/extension-firefox.zip" . -x "*.DS_Store" -x "extension.zip" -x "STORE_DESCRIPTION.md"
)

(
  cd "${ROOT}/assets/fonts"
  zip -r -9 "${OUT_DIR}/fonts.zip" JetBrainsMonoNerdFont -x "*.DS_Store"
)
