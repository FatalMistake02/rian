# Configuration

## Local files

Run `rian config` to find your config file.

| Platform | Default config path |
| --- | --- |
| Windows | `%AppData%\rian\config.json` |
| Linux | `$XDG_CONFIG_HOME/rian/config.json`, or `~/.config/rian/config.json` |
| macOS | `~/Library/Application Support/rian/config.json` |

`RIAN_CONFIG_HOME` overrides the config directory. UI errors are logged to `rian.log` beside the config. If `config.json.lock` remains after a crash, close all Rian instances before removing it.

## Settings

Settings covers appearance, startup behavior, terminal preferences, tool and command visibility, folder discovery, and addons. Projects can override global shortcut and command visibility.

Open **Settings → section order** to rearrange project details. Select a section with ↑/↓, then use ←/→ or Ctrl+↑/↓ to move it up or down. Each metadata addon has its own movable section alongside the tech stack, GitHub, Git status, last commit, and commands. Changes preview immediately; Ctrl+S saves, Ctrl+C cancels, and Backspace restores the default order. This order applies across projects. Project name/path, shortcut hints, and the separate Git graph and notes panels keep their positions. Newly installed addon sections are appended to a saved order.

## Detection and commands

Rian detects Node.js, Go, Rust, Python, and Flutter projects from their root manifests. A declared package manager takes priority over lockfiles. Custom commands override detected commands by name.

Project commands use `cmd.exe` on Windows and `$SHELL` (or `/bin/sh`) on Linux/macOS.

New terminal windows support Konsole, GNOME Terminal, and xterm on Linux; Windows Terminal on Windows; and Terminal on macOS.

## App launcher

App shortcuts are stored in `~/.local/share/applications/rian.desktop` on Linux (or under `XDG_DATA_HOME`), `~/Applications/Rian.app` on macOS, and the Start menu on Windows. They point to the installed native executable; moving that installation requires recreating the shortcut.

## Updates

For a global npm installation:

```sh
npm install -g @fatalmistake02/rian@latest
```

Restart Rian after updating.

Addons with an `update_url` check for new versions in the background. Available updates are installed only when selected. See [addon updates](addons.md#versions-and-updates).
