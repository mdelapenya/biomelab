# Installation

## macOS (Homebrew cask)

```bash
brew install --cask mdelapenya/tap/biomelab
```

This installs `Biomelab.app` to `/Applications` — find it in Spotlight.

## macOS (manual)

Download `Biomelab-darwin-universal.zip` from the [latest release](https://github.com/mdelapenya/biomelab/releases/latest), unzip, and drag `Biomelab.app` to `/Applications`.

> **Gatekeeper note:** The macOS app is ad-hoc signed, but is not Developer ID signed or notarized. Run `xattr -d com.apple.quarantine /Applications/Biomelab.app` if macOS blocks it.

## Linux

Download `Biomelab-linux-amd64.tar.xz` from the [latest release](https://github.com/mdelapenya/biomelab/releases/latest) and extract. The Fyne archive contains an application layout under a top-level directory; the executable is in `usr/local/bin/biomelab` within that layout. A graphical desktop and runtime graphics libraries are required.

## Windows

Download `Biomelab-windows-amd64.zip` from the [latest release](https://github.com/mdelapenya/biomelab/releases/latest), extract, and run `Biomelab.exe`.

## Nightly builds

```bash
brew install --cask mdelapenya/tap/biomelab-nightly
brew reinstall --cask mdelapenya/tap/biomelab-nightly  # update to latest
```

Or download from the [releases page](https://github.com/mdelapenya/biomelab/releases) — look for `v<version>-nightly`.

## From source

Use the Go version declared in [go.mod](../go.mod) (currently 1.25.6).

Requires [Git](https://git-scm.com/), [Task](https://taskfile.dev/), and a C toolchain (Fyne uses CGO). Everything else — `gvm`, the Go toolchain, and the `fyne` CLI — is bootstrapped by `task setup` / `task setup-fyne`.

```bash
# Install a C toolchain for your OS
#   macOS:    xcode-select --install
#   Linux:    sudo apt install gcc libgl1-mesa-dev xorg-dev
#   Windows:  scoop install gcc           (PowerShell; or MSYS2 / WinLibs)

git clone https://github.com/mdelapenya/biomelab.git
cd biomelab
task build          # builds bin/biomelab
task install        # installs to $GOPATH/bin
task install-macos  # builds universal .app + installs to /Applications (macOS only)
```

## Optional integrations

Install only the tools needed for your workflow. The tray's **Dependencies** dialog shows availability, version, authentication problems, install hints, and links; use **Re-check** after fixing a dependency.

| Tool | Enables |
|---|---|
| `gh`, authenticated with `gh auth login` | GitHub status, PR creation and PR checkout; sandbox kit catalog discovery |
| `glab`, authenticated with `glab auth login` | GitLab MR status and creation |
| `sbx`, initialized with `sbx ls` | Docker Sandbox lifecycle and sessions |
| `rgt` | Optional re_gent activity integration and JSON export |

See [sandbox setup](sandboxes.md) and [configuration and troubleshooting](configuration.md) for platform-specific behavior. Installing a packaged app does not require Go or a C compiler.

## Ignore generated worktrees

Add `.biomelab-worktrees/` and `.sbx/` to your configured global Git ignore file. Check `git config --get core.excludesFile` before choosing a file; if unset, Git normally uses `$XDG_CONFIG_HOME/git/ignore` or `~/.config/git/ignore`. Create its parent directory if necessary. Notes and re_gent data are excluded automatically through the repository's `info/exclude`.

## Nix

The repository's flake is unfinished (including a placeholder `vendorHash`). Use a packaged release or the source build above until the flake has been repaired and verified.
