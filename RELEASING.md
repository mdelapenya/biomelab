# Releasing

biomelab uses GitHub Actions for automated builds and releases.

## Pipelines

| Workflow | Trigger | What it does |
|----------|---------|--------------|
| **CI** (`ci.yml`) | Push to `main`, PRs | Lint, test, build |
| **Nightly** (`nightly.yml`) | After CI passes on `main` | Build all platforms, publish pre-release, update nightly cask |
| **Release** (`release.yml`) | Tag push `v*` | Build all platforms, publish release, update stable cask |

## Platforms

All pipelines build for three platforms:

| Platform | Runner | Artifact | How it's built |
|----------|--------|----------|----------------|
| macOS (Universal) | `macos-latest` | `Biomelab-darwin-universal.zip` | `task package-darwin-universal` — builds arm64 + amd64, merges with `lipo`, packages as `.app` |
| Linux (amd64) | `ubuntu-latest` | `Biomelab-linux-amd64.tar.xz` | `task package` — `fyne package` for current OS |
| Windows (amd64) | `windows-latest` | `Biomelab-windows-amd64.zip` | `task package` — `fyne package` for current OS |

### Build dependencies

- **macOS**: Xcode Command Line Tools (pre-installed on runners). `lipo` for universal binaries.
- **Linux**: `gcc libgl1-mesa-dev xorg-dev` (installed in workflow).
- **Windows**: MinGW GCC (pre-installed on `windows-latest` runners).
- **All**: Go (from `go.mod`), Task CLI, fyne CLI.

## Stable release

To cut a release:

```bash
git tag v1.0.0
git push origin v1.0.0
```

The `release.yml` workflow:

1. Builds macOS universal `.app`, Linux `.tar.xz`, Windows `.exe` (all via `task`)
2. Creates a GitHub Release with all three artifacts
3. Computes SHA256 of the macOS zip
4. Pushes a `biomelab` cask to `mdelapenya/homebrew-tap` pointing to the release asset

Users install via:

```bash
# macOS (Homebrew cask)
brew install --cask mdelapenya/tap/biomelab

# macOS (manual)
# Download Biomelab-darwin-universal.zip from the release, unzip, drag to /Applications

# Linux
# Download Biomelab-linux-amd64.tar.xz from the release, extract, run

# Windows
# Download Biomelab-windows-amd64.zip from the release, extract, run Biomelab.exe
```

## Nightly builds

After every successful CI run on `main`, the `nightly.yml` workflow:

1. Computes a nightly tag: `v<latest-stable>-nightly` (e.g., `v1.0.0-nightly`)
2. Deletes the previous nightly tag/release (rolling)
3. Builds all three platforms
4. Creates a pre-release on GitHub
5. Pushes a `biomelab-nightly` cask to the homebrew tap (conflicts with stable)

Users install via:

```bash
brew install --cask mdelapenya/tap/biomelab-nightly
brew reinstall --cask mdelapenya/tap/biomelab-nightly  # update to latest
```

## Local packaging

### macOS

```bash
task install-macos  # builds universal binary, packages .app, copies to /Applications
```

This runs `task package-darwin-universal` which:
1. `task build-darwin-arm64` + `task build-darwin-amd64` (parallel)
2. `lipo -create` → universal binary
3. `fyne package --executable` → `Biomelab.app`
4. `cp -R` to `/Applications`

### Current platform

```bash
task package  # produces .app (macOS), .tar.xz (Linux), or .exe (Windows) in bin/
```

### Build only (no packaging)

```bash
task build  # produces bin/biomelab for current OS/arch
```

## Homebrew tap

The tap lives at [mdelapenya/homebrew-tap](https://github.com/mdelapenya/homebrew-tap).

| Cask | Formula | What it installs |
|------|---------|-----------------|
| `biomelab` | `Casks/biomelab.rb` | Stable release `.app` |
| `biomelab-nightly` | `Casks/biomelab-nightly.rb` | Nightly `.app` (conflicts with stable) |

Both casks are updated automatically by GitHub Actions using `HOMEBREW_TAP_GITHUB_TOKEN`.

## Versioning

- **Git tags**: `v1.0.0` (stable), `v1.0.0-nightly` (nightly)
- **Binary version**: set via `-ldflags "-X main.version=..."` — includes full git describe output
- **Fyne app version**: stripped to semver `x.y.z` (fyne rejects anything else)
- **FyneApp.toml**: metadata for `fyne package` (icon path, app ID, name)

## Icon

The app icon lives at `cmd/biomelab/icon.png`. It's:
- Embedded in the binary via `//go:embed` for window icon and system tray
- Used by `fyne package` via `--icon cmd/biomelab/icon.png` for `.app` bundle and `.exe` resource embedding
- Referenced in `FyneApp.toml`

Single source of truth — no duplicates.
