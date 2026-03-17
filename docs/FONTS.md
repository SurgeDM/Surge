# Fonts

Surge ships a bundled Nerd Font so you can get a consistent TUI look and
glyph coverage out of the box. The app itself cannot force a terminal font;
you must install the font locally and select it in your terminal emulator.

## Bundled Font

Surge includes JetBrains Mono Nerd Font Mono (Regular, Bold, Italic, Bold Italic).
Download `fonts.zip` from the latest GitHub release.

## Install

### macOS

1. Unzip `fonts.zip`.
2. Double-click the TTF files and click Install in Font Book.
3. Set your terminal font to `JetBrainsMono Nerd Font Mono`.

### Linux

1. Unzip `fonts.zip`.
2. Copy the TTF files to `~/.local/share/fonts/` (or `~/.fonts/`).
3. Run `fc-cache -f`.
4. Set your terminal font to `JetBrainsMono Nerd Font Mono`.

### Windows

1. Unzip `fonts.zip`.
2. Right-click each TTF file and choose Install.
3. Set your terminal font to `JetBrainsMono Nerd Font Mono`.

## License

JetBrains Mono Nerd Font is distributed under the SIL Open Font License 1.1.
See `OFL.txt` and `NOTICE.md` inside `fonts.zip` for details.
