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

## Documentation and website checks

Before tagging a release:

- Compare download names and architectures in [installation](docs/installation.md) and `website/index.html` with the release workflow artifacts.
- Check prerequisites against `go.mod` and `Taskfile.yml`; distinguish packaged installation from source builds.
- Walk through the [dashboard](docs/dashboard.md), sandbox setup, notes, and activity guides in the GUI. Compare shortcuts with `internal/gui/shortcuts.go` and dialog controls.
- Revisit [known limitations](docs/known-limitations.md) and remove resolved items only after verification.
- Run the Lychee command below and `node --check website/js/main.js`. CI uses [lychee-action](https://github.com/lycheeverse/lychee-action) pinned to a full commit SHA, with a fixed Lychee version. These checks validate local links/anchors and JavaScript syntax, not external service availability.
- Serve `website/` locally (`python3 -m http.server 8765 --directory website`) and check desktop/mobile layouts, both themes, installation tabs, and playground note Save/Cancel/Delete, PR flow, and sandbox setup. Keep simulated actions labeled.
- Update GUI screenshots when the layout changes. Use sample data and label it; do not expose personal repositories or activity logs.
- Check external installation/documentation links before publishing. Website deployment runs on pushes to `main` affecting `website/**`; docs links on the website target `main`, so publish the linked guides alongside the website change.

### Check documentation links locally

Install [Lychee](https://lychee.cli.rs/) v0.24.2 to match CI, then run from the repository root:

```bash
lychee --offline --include-fragments --no-progress \
  --root-dir "$PWD/website" \
  --remap "^https://github\.com/mdelapenya/biomelab/blob/main/ file://$PWD/" \
  '*.md' 'docs/**/*.md' '.claude/**/*.md' 'website/**/*.html'
```

The remapping checks this repository's GitHub `blob/main` links against the
local checkout, so new guides can pass before merging. Offline mode skips
external websites; fragment checking validates local heading and HTML anchors.

### Refresh dashboard images

The helper at `cmd/helpers/screenshot-generator` renders `NewRepoPanel` and
`NewDashboard` in Fyne's offscreen test canvas with fictional paths and PRs.
It opens no desktop window and does not load your repositories or config.

From the repository root, with the [source-build prerequisites](docs/installation.md#from-source):

```bash
go run ./cmd/helpers/screenshot-generator
```

This creates or overwrites `website/img/dashboard-dark.png` and
`website/img/dashboard-light.png`. To inspect new images before replacing the
website assets, use a separate directory:

```bash
go run ./cmd/helpers/screenshot-generator -output-dir /tmp/biomelab-screenshots
```

It can also be built as a standalone binary:

```bash
go build -o bin/screenshot-generator ./cmd/helpers/screenshot-generator
```

The output directory is relative to the working directory unless an absolute
path is provided. No build tag is needed. CI's `task test-race` runs the helper's
normal Go tests too: they render both themes, decode the PNGs, check dimensions and nonblank output, and exercise CLI
and filesystem error handling. All test images go into temporary directories;
only an explicit helper invocation writes the website assets.

To run just the helper tests locally:

```bash
go test -race ./cmd/helpers/screenshot-generator
```

These images show application widgets, not a capture of a user's desktop;
preserve that distinction in their captions. Inspect both images before
committing them.

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
3. `fyne package --executable` with an absolute binary path → `Biomelab.app`
4. Ad-hoc sign the bundle and run `task verify-darwin-universal`
5. `cp -R` to `/Applications` (with `task install-macos`)

The verification reads `CFBundleExecutable` from the finished app, requires both
`x86_64` and `arm64` slices, checks the signature, and runs `--version` on the host.
CI, nightly, and release builds use this check before accepting the package.
The absolute executable path is required because Fyne changes into `--source-dir`;
a relative path can cause it to rebuild for only the host architecture.

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
