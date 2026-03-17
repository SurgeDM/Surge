#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT}/.goreleaser-extra"

if ! command -v zip >/dev/null 2>&1; then
  echo "zip is required to package release assets" >&2
  exit 1
fi
if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required to download Nerd Fonts assets" >&2
  exit 1
fi
if ! command -v tar >/dev/null 2>&1; then
  echo "tar is required to extract Nerd Fonts assets" >&2
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

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

NERD_FONTS_VERSION="v3.4.0"
NERD_FONTS_SHA256="ef552a3e638f25125c6ad4c51176a6adcdce295ab1d2ffacf0db060caf8c1582"
FONT_ARCHIVE="${TMP_DIR}/JetBrainsMono.tar.xz"
curl -L -o "${FONT_ARCHIVE}" \
  "https://github.com/ryanoasis/nerd-fonts/releases/download/${NERD_FONTS_VERSION}/JetBrainsMono.tar.xz"
if command -v sha256sum >/dev/null 2>&1; then
  echo "${NERD_FONTS_SHA256}  ${FONT_ARCHIVE}" | sha256sum -c -
else
  echo "${NERD_FONTS_SHA256}  ${FONT_ARCHIVE}" | shasum -a 256 -c -
fi
tar -xf "${FONT_ARCHIVE}" -C "${TMP_DIR}"

FONT_DIR="${TMP_DIR}/JetBrainsMonoNerdFont"
mkdir -p "${FONT_DIR}"
cp "${TMP_DIR}/JetBrainsMonoNerdFontMono-Regular.ttf" "${FONT_DIR}/"
cp "${TMP_DIR}/JetBrainsMonoNerdFontMono-Bold.ttf" "${FONT_DIR}/"
cp "${TMP_DIR}/JetBrainsMonoNerdFontMono-Italic.ttf" "${FONT_DIR}/"
cp "${TMP_DIR}/JetBrainsMonoNerdFontMono-BoldItalic.ttf" "${FONT_DIR}/"
cp "${TMP_DIR}/OFL.txt" "${FONT_DIR}/"

cat > "${FONT_DIR}/NOTICE.md" <<'EOF'
JetBrains Mono Nerd Font (Mono) files in this folder are derived from
JetBrains Mono and patched by the Nerd Fonts project.

Source:
- Nerd Fonts (ryanoasis/nerd-fonts) release assets: JetBrainsMono.tar.xz

License:
- SIL Open Font License 1.1 (see OFL.txt in this folder)

These files are redistributed as permitted by the OFL. No Reserved Font Name
applies to this font (per Nerd Fonts license audit).
EOF

(
  cd "${TMP_DIR}"
  zip -r -9 "${OUT_DIR}/fonts.zip" JetBrainsMonoNerdFont -x "*.DS_Store"
)
