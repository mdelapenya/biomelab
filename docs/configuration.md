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
| `BIOME_TERMINAL` | Terminal executable accepting `-e sh -c` | macOS default terminal / Linux `x-terminal-emulator` |

GUI launches from Finder/Spotlight need not inherit your shell's environment. BiomeLab expands common CLI search paths, but configure environment overrides in the environment that launches the app.

## Troubleshooting

| Symptom | What to check |
|---|---|
| Missing PR/MR status | Open tray → Dependencies; authenticate the appropriate `gh` or `glab` CLI, then Re-check. GitHub and GitLab are supported; custom GitLab hosts are recognized when their hostname contains `gitlab.`. |
| GitLab MR checkout fails | `f` currently uses GitHub PR checkout. GitLab status and creation are separate supported operations. |
| Sandbox setup fails | Run `sbx ls` in a terminal and complete setup. Kit discovery additionally needs working `gh` access. |
| Existing terminal does not activate | macOS activation uses AppleScript/TTY matching (Terminal.app/iTerm2) and may require Automation permission. Linux uses `xdotool`, with a `wmctrl` fallback; desktop/session support varies. |
| Terminal opening on Windows | There is no native default launcher. A `BIOME_TERMINAL` override must accept `-e sh -c`, with `sh` available. This is not general support for arbitrary Windows terminal commands. |
| Editor does not open | Check `BIOME_EDITOR` and PATH; on macOS the app also attempts an application-name fallback. |
| Activity log is empty or unavailable | Install host `rgt` and check that the worktree has recorded re_gent activity. See the sandbox distinction in the activity guide. |
| Closing the app leaves it running | Closing hides the window. Use tray → Quit to exit. |

See [known limitations](known-limitations.md) for discrepancies tracked during documentation review.
