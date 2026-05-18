# Custom Themes Guide

Surge features a powerful palette-based theme engine that allows you to customize every aspect of the TUI's appearance using simple `.toml` files.

## Theme Resolution

Surge looks for themes in the following order:

1.  **Direct Path / Working Directory**: If you provide a full path or a filename in your current directory (including autocomplete results).
2.  **Local `themes/` Folder**: Surge looks for themes in a `themes/` directory relative to where it is running (useful for bundled themes).
3.  **Global Surge Themes**: Surge then checks for themes in your user configuration directory:
    *   **Linux**: `~/.config/surge/themes/`
    *   **macOS**: `~/Library/Application Support/surge/themes/`
    *   **Windows**: `%APPDATA%\surge\themes\`

## Theme File Format

A Surge theme is a TOML file containing a `[colors]` table. The system uses a 16-color palette (Normal + Bright) plus primary background and foreground colors.

### Basic Structure

```toml
[colors]
name = "My Custom Theme"

[colors.primary]
background = "#1a1b26"
foreground = "#a9b1d6"

[colors.normal]
black   = "#32344a"
red     = "#f7768e"
green   = "#9ece6a"
yellow  = "#e0af68"
blue    = "#7aa2f7"
magenta = "#ad8ee6"
cyan    = "#449dab"
white   = "#787c99"

[colors.bright]
black   = "#444b6a"
red     = "#ff7a93"
green   = "#b9f27c"
yellow  = "#ff9e64"
blue    = "#7da6ff"
magenta = "#bb9af7"
cyan    = "#0db9d7"
white   = "#acb0d0"
```

### Mapping Details

Surge maps these terminal colors to specific UI elements:

| Color Group | Color | Surge UI Mapping |
| :--- | :--- | :--- |
| **Normal** | `black` | Selection backgrounds, Borders, Secondary text |
| | `red` | Error states |
| | `green` | Active download state |
| | `yellow` | Paused state |
| | `blue` | Version info, Action hints |
| | `magenta` | Completed state |
| | `cyan` | Setting labels, Links |
| | `white` | Standard text |
| **Bright** | `black` | Secondary info, Timestamps |
| | `red` | Progress bar start (Gradient) |
| | `magenta` | Progress bar end (Gradient) |

---

## Adaptive Themes

You can bundle both Dark and Light variants into a single theme file. Surge will automatically switch between them based on your `theme` setting (Adaptive/Light/Dark).

```toml
[colors]
name = "My Adaptive Theme"

# Default colors used if mode-specific ones are missing
[colors.primary]
background = "#000000"
foreground = "#ffffff"

[colors.dark]
[colors.dark.primary]
background = "#1a1b26"
# ... other dark colors

[colors.light]
[colors.light.primary]
background = "#ffffff"
# ... other light colors
```

## Applying a Theme

1.  Place your `.toml` file in the `themes` directory.
2.  Open Surge TUI.
3.  Go to **Settings** (`s`).
4.  Navigate to **Theme File** (`theme_path`).
5.  Type the name of your theme (without `.toml`) or press `Tab` to browse and select it.
