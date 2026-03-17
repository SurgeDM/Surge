# Fonts

Surge ships a bundled Nerd Font so you can get a consistent TUI look and
glyph coverage out of the box. The app itself cannot force a terminal font;
you must install the font locally and select it in your terminal emulator.

## Bundled Font

Surge includes JetBrains Mono Nerd Font Mono (Regular, Bold, Italic, Bold Italic).
The font files live in:

`assets/fonts/JetBrainsMonoNerdFont/`

## Install

### macOS

1. Open `assets/fonts/JetBrainsMonoNerdFont/`.
2. Double-click the TTF files and click Install in Font Book.
3. Set your terminal font to `JetBrainsMono Nerd Font Mono`.

### Linux

1. Copy the TTF files to `~/.local/share/fonts/` (or `~/.fonts/`).
2. Run `fc-cache -f`.
3. Set your terminal font to `JetBrainsMono Nerd Font Mono`.

### Windows

1. Open `assets/fonts/JetBrainsMonoNerdFont/`.
2. Right-click each TTF file and choose Install.
3. Set your terminal font to `JetBrainsMono Nerd Font Mono`.

## License

JetBrains Mono Nerd Font is distributed under the SIL Open Font License 1.1.
See `assets/fonts/JetBrainsMonoNerdFont/OFL.txt` and
`assets/fonts/JetBrainsMonoNerdFont/NOTICE.md` for details.
