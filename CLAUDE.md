# CLAUDE.md -- Project context for AI agents

biomelab is a Go desktop GUI (Fyne) that manages git worktrees for AI coding
agents. Multi-repo dashboard with sandbox (Docker) and regular modes.

For user-facing behavior, start with the [user guides](README.md#user-guides).
For product decision rationale, see [.claude/skills/product-owner/SKILL.md](.claude/skills/product-owner/SKILL.md).
For Fyne framework reference and pitfalls, see [.claude/skills/fyne-developer/SKILL.md](.claude/skills/fyne-developer/SKILL.md).
For internal architecture, design decisions, and state machines, see [ARCHITECTURE.md](ARCHITECTURE.md).

## Build and test commands

Fyne requires CGo and system graphics libraries.

```
# Prerequisites — install a C toolchain
#   macOS:    xcode-select --install
#   Linux:    sudo apt install gcc libgl1-mesa-dev xorg-dev
#   Windows:  scoop install gcc    (or MSYS2: pacman -S mingw-w64-ucrt-x86_64-gcc)

# Build and test
task build        # Build to bin/biomelab (CGO_ENABLED=1)
task install      # Install to $GOPATH/bin
task test         # Run tests
task test-race    # Run tests with -race
task lint         # Run golangci-lint
task clean        # Remove build artifacts

# Packaging
task package                 # Package for current OS via fyne
task package-darwin-universal # macOS universal binary (.app bundle)
task install-macos            # Package + install to /Applications
```

Go version: see `go.mod` (currently 1.25.6). Check `go env GOROOT` if you hit version mismatches.

## Key dependencies

- **Fyne v2.7** -- Desktop GUI framework. See `/fyne-developer` skill.
- **go-git v6** (unreleased, from main branch) -- All git operations.
- **gopsutil** -- Cross-platform process detection.
- **gh CLI / glab CLI** -- External tools for PR/MR status.
- **rgt CLI** ([re_gent](https://github.com/regent-vcs/re_gent)) -- Optional. When installed, biomelab auto-initializes `.regent/` per worktree and wires Claude Code hooks.

## Package layout

```
cmd/biomelab/          Entry point, icon embedding, PATH expansion
internal/
  gui/                 Fyne GUI: app, dashboard, cards, repo panel, dialogs,
                       keyboard handling, theme, refresh manager, system tray,
                       sysdeps dialog + banner, regent log window, OS-native
                       save dialog helper, kanban, theme persistence, repo dragging
  ops/                 Shared business operations (refresh, worktree CRUD,
                       sandbox ops), independent of GUI widgets
  config/              OS-specific repo, mode, kit, order, and theme persistence
  git/                 Go-git v6 wrapper + EnsureExcluded helper
  agent/               Agent process detection
  ide/                 IDE process detection
  process/             Shared process enumeration
  provider/            PR/MR provider abstraction (GitHub, GitLab)
  sandbox/             Docker Sandbox (sbx) CLI wrapper
  kits/                Compatible kit discovery + Docker Hub OCI references
  notes/               Per-worktree Markdown notes (.biomelab/note.md)
  regent/              re_gent (rgt) integration: detect, init, hooks, log
  sysdeps/             External CLI dependency checks (gh/glab/sbx/rgt)
  terminal/            Terminal detection, launch, and activation
  github/              GitHub-specific PR helpers
```

## Testing patterns

- **Config tests**: `t.TempDir()` for config paths. Round-trip save/load, dedup, remove.
- **Git tests**: Real temp repos via `t.TempDir()` + `gogit.PlainInit`.
- **Agent/IDE tests**: Mock `process.Lister` interface with canned `process.Info` slices.

Always run `go test -race ./...`.

## Fyne GUI pitfalls

- **No Focusable widgets in the content tree** — `widget.Tree` steals keyboard
  focus. We use tappable VBox items instead. See `/fyne-developer` skill.
- **`desktop.Canvas.SetOnKeyDown`** is the only way to receive Tab/Escape
  globally. `Canvas.SetOnTypedKey` fires too late (after Tab is consumed).
- **Do NOT set both `SetOnKeyDown` and `SetOnTypedKey`** — they double-fire
  on initial press.
- **Dialog dismissal via `Hide()`** — never remove overlays manually
  (`overlays.Remove`) as it corrupts Fyne's canvas state.
- **`fyne.KeyEvent` has no modifier info** — Shift+S and S both arrive as
  `fyne.KeyS`. Use `SetOnTypedRune` for case-sensitive keys ('S' vs 's').
- **macOS GUI PATH is minimal** — `init()` in main.go expands PATH to include
  `/usr/local/bin`, `/opt/homebrew/bin`, `~/.docker/bin` etc.
- **`sandbox.StatusNotFound` is 0** (iota) — don't use `!= 0` as a guard.
  Use `HasSbxStatus bool` flag instead.
- **`canvas.Text` doesn't clip** — truncate strings manually. Paths use
  prefix truncation (`truncatePath`), PR text uses suffix truncation.
- **Card grid navigation** — up/down jump by column count, left/right by 1.
  Column count computed from `dashSlot.Size().Width / cardCellSize().Width`.
- **`rgt init` ignores positional path args** — it always operates on `cwd`.
  Use `cmd.Dir = wtPath`, not `rgt init <wtPath>`.
- **rgt hook installer needs a TTY** — `--agent claude` doesn't skip the
  prompt in a non-TTY environment. Biomelab writes `.claude/settings.json`
  itself via `regent.EnsureClaudeHooks` (ported from rgt upstream, Apache-2.0).
- **sbx has no `--branch` any more** — sandbox-mode worktrees are plain host
  worktrees under `.biomelab-worktrees/` (sbx mirrors the mount path, so they
  exist in-container at the same path on macOS/Linux; on Windows `C:\a\b` is
  mounted at `/c/a/b`, so host paths must go through `sandbox.ContainerPath`
  before reaching sbx). Linked cards attach with
  `sbx exec -it -w <path> <sbx> bash -c '…start-agent…'`; the main card uses
  `sbx run --name <sbx>`. See `sandbox.ExecAgentArgs`.
- **`widget.Accordion` misbehaves inside `VScroll`** — clicks don't toggle.
  Use a `widget.Button` that flips the inner container's visibility instead;
  see the regent log dialog's tools collapsible for the pattern.
- **One regent log window at a time** — keyed by `App.regentLogWindow` plus
  a `regentLogReload` closure. Different cards reuse the same window via
  reload; don't spawn a new one per worktree.

## Release process

See [RELEASING.md](RELEASING.md) for the full release workflow.
