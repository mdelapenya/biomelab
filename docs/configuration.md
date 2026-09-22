# Configuration and troubleshooting

## Saved settings

Use tray → **Show Config** to open the actual file. BiomeLab uses Go's OS-specific user configuration directory:

| Platform | Default path |
|---|---|
| macOS | `~/Library/Application Support/biomelab/repos.json` |
| Linux | `$XDG_CONFIG_HOME/biomelab/repos.json`, or `~/.config/biomelab/repos.json` |
| Windows | `%AppData%\biomelab\repos.json` |

The file stores repository order, paths, modes, sandbox kit metadata, and theme (`dark` by default or `light`). Old flat mode entries and the former `gwaim` configuration are migrated on load. View mode and zoom are session settings.

## Command-line options

The `biomelab` command launches the desktop GUI.

| Option | Short form | Behavior |
|---|---|---|
| `--version` | `-v` | Print version and exit |
| `--refresh 1m` | `-r 1m` | Set network refresh interval |

Network interval precedence is CLI flag, then `BIOME_REFRESH`, then `30s`. Choose a positive duration such as `30s` or `1m`. Local refresh remains five seconds.

| Environment variable | Behavior | Default |
|---|---|---|
| `BIOME_REFRESH` | Network refresh duration | `30s` |
| `BIOME_EDITOR` | Editor executable (a command name, not a shell command with arguments) | `code` |
| `BIOME_TERMINAL` | On macOS/Linux, terminal executable accepting `-e sh -c`. On Windows, a `wt`/`wt.exe`, `pwsh`/`pwsh.exe`, or `powershell`/`powershell.exe` executable path; other recipes are rejected. | macOS default terminal / Linux `x-terminal-emulator`; on Windows, `wt.exe` when available, otherwise `pwsh.exe` then `powershell.exe` |

GUI launches from Finder/Spotlight need not inherit your shell's environment. BiomeLab expands common CLI search paths, but configure environment overrides in the environment that launches the app.

## Troubleshooting

| Symptom | What to check |
|---|---|
| Missing PR/MR status | Open tray → Dependencies; authenticate the appropriate `gh` or `glab` CLI, then Re-check and trigger a dashboard refresh (`r`) to recheck its authentication state. GitHub and GitLab are supported; custom GitLab hosts are recognized when their hostname contains `gitlab.`. |
| GitLab MR checkout fails | `f` currently uses GitHub PR checkout. GitLab status and creation are separate supported operations. |
| Sandbox setup fails | Run `sbx ls` in a terminal and complete setup. Kit discovery additionally needs working `gh` access. |
| Existing terminal does not activate | macOS activation uses AppleScript/TTY matching (Terminal.app/iTerm2) and may require Automation permission. Linux requires X11 and `xdotool`, plus a launch-time window ID or a unique emulator window. On Windows, a tracked classic PowerShell console can be focused only when its recorded visible HWND and exact title still match. Windows Terminal is deliberately not focused: a repeat Enter reports a manual-switch error while retaining the live session, so it does not launch a duplicate. Other macOS emulators and Wayland also require switching manually. |
| Terminal startup command loses characters or fails | Interactive login-shell prompts (for example an Oh My Zsh update prompt) can consume the `.command` path queued by Terminal.app. Complete or disable that startup prompt, close the failed terminal, and use the retry flow below. |
| Terminal remains “still starting” | Wait for startup and press Enter again. After 30 seconds, an unresolved launch offers **Forget session**. Check for and close the previous terminal before forgetting; the next Enter can then retry. |
| Terminal opening on Windows | BiomeLab prefers Windows Terminal (`wt.exe`). Otherwise, a hidden PowerShell helper starts the selected `pwsh.exe` or `powershell.exe` in its own visible interactive console, preserving usable console handles from a GUI launch. It removes inherited `WT_SESSION`/`WT_PROFILE_ID` values and records the actual final host: Windows may still delegate PowerShell to Windows Terminal under the OS default-terminal setting. Set `BIOME_TERMINAL` only to one of those known executables (a path is allowed); arbitrary terminal recipes are rejected. Native desktop acceptance remains pending—follow the [Windows focus verification guide](windows-focus-validation.md). |
| Windows repeatedly loses keyboard focus | Background helpers run without creating a console window. If interruptions persist, use the [Windows focus verification guide](windows-focus-validation.md) to identify which window takes focus. |
| Editor does not open | Check `BIOME_EDITOR` and PATH; on macOS the app also attempts an application-name fallback. |
| Activity log is empty or unavailable | Install host `rgt` and check that the worktree has recorded re_gent activity. See the sandbox distinction in the activity guide. |
| Closing the app leaves it running | Closing hides the window. Use tray → Quit to exit. |

See [known limitations](known-limitations.md) for discrepancies tracked during documentation review.
