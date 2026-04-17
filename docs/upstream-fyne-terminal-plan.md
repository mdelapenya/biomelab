# Upstream contribution plan — `github.com/fyne-io/terminal`

Snapshot date: 2026-04-17. Repo HEAD at snapshot: `73c54fb` (post-merge of PR #144). Dep pinned in biomelab `go.mod`: `v0.0.0-20260406081141-73c54fbd1dd3`.

Goal: land upstream fixes that let us delete the workarounds currently living in `internal/gui/termpanel.go` and `internal/gui/keycapture.go` (scroll forwarder, modified-arrow shortcuts, resize-listener dedup).

---

## 0. Repo overview & house rules

### Layout
```
github.com/fyne-io/terminal/
├── term.go              widget struct, Resize, AddListener/RemoveListener, onConfigure
├── term_unix.go         startPTY for macOS/Linux (creack/pty, SHELL env, TERM=xterm-256color)
├── term_windows.go      ConPTY variant
├── input.go             TypedRune, TypedKey, TypedShortcut, trackKeyboardState, keyTypedWithShift, typeCursorKey
├── output.go            asciiBackspace=8, asciiEscape=27
├── escape.go, dcs.go, apc.go, osc.go, render.go, mouse.go, select.go, color.go, position.go
├── *_test.go            standard Go table-driven tests; no testify
├── cmd/fyneterm/        the reference app
├── internal/widget/     internal TextGrid wrappers
└── .github/workflows/   CI (Platform Tests, Static Analysis)
```

### CI requirements (must all pass)

**`.github/workflows/platform_tests.yml`:**
- Matrix: `{go-version: ["", "stable"]} × {ubuntu-latest, macos-latest}`
- Command: `go test -tags ci -covermode=atomic -coverprofile=coverage.out ./...`
- **Coverage gate: total must stay `>= 13%`** (enforced only on Linux + stable Go). Any PR that drops coverage below this floor is rejected.
- Updates coveralls on merge.

**`.github/workflows/static_analysis.yml`:**
- `go vet -tags ci ./...`
- `goimports -e -d .` must be clean
- `gocyclo -over 30 .` must be clean (no function > cyclomatic 30)
- `golint -set_exit_status $(go list -tags ci ./...)`
- `staticcheck -go 1.19 ./...` (pinned to v0.4.2, targets Go 1.19 — so no Go 1.21+ syntax)

There is **no `CONTRIBUTING.md`** in the repo. The README points to the `#fyne` channel on Gophers Slack (`http://gophers.slack.com/messages/fyne`) — use it for design questions before large PRs.

### Code style observed
- Plain Go; no testify; tests use `map[string]struct{...}` tables + `bytes.Equal`.
- Exported funcs have one-line docstrings starting with the function name.
- Private helpers live next to their callers (no `util.go`-style dumping grounds).
- Fyne v2 is the only UI dep. `creack/pty` for Unix, `ActiveState/termtest/conpty` for Windows.
- Mock pattern for input tests: `term := &Terminal{in: NopCloser(inBuffer)}` — direct struct init, no constructor.

### Key maintainer
**Andy Williams (`andydotxyz`)** is the Fyne project lead and primary reviewer. Most merges are his direct PRs or his reviews. Engaging with him on Slack before non-trivial PRs is the path of least resistance.

### Open PRs that may conflict
| # | Title | Author | Status | Overlap? |
|---|---|---|---|---|
| 147 | Add scroll support | andydotxyz | OPEN, 7 files, +259/-59 | **Yes** — actual scrollback via `rowOffset()`. Our Fyne scroll-wheel PR must wait or coordinate. |
| 148 | Fixes #140 refresh interval | pneumaticdeath | OPEN | No. Different concern. |
| 146 | Monospace font styles | andydotxyz | OPEN | Unlikely — touches render.go font path. |
| 136 | Debounce resize | figuerom16 | **CLOSED** (not merged) | Relevant history for resize perf. No review comments — maintainer didn't explain. Ask before re-attempting. |

### Open issues this plan addresses
| # | Title | Relevance |
|---|---|---|
| 103 | Allow forwarding resize events | `RunWithConnection` resize hook we worked around via `AddListener`. |
| 6 | Connections through `RunWithConnection` do not get resize information | Same as #103, predates it. Labeled "bug". |
| 138 | Backspace not handled correctly (`^H` vs `^?`) | Adjacent to PR 1 (input.go); can be bundled or separate. |
| 32 | Add scrollback | Out of scope — too large; being addressed by #147. |
| 78 | Resize refresh glitch | Adjacent to the resize-perf concern. |

---

## 1. Prerequisites

Before touching code:

1. Fork `fyne-io/terminal` to your GitHub account.
2. Clone:
   ```bash
   git clone git@github.com:<you>/terminal.git
   cd terminal
   git remote add upstream https://github.com/fyne-io/terminal
   git fetch upstream
   ```
3. Install the static-analysis tools CI uses:
   ```bash
   go install golang.org/x/tools/cmd/goimports@latest
   go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
   go install golang.org/x/lint/golint@latest
   go install honnef.co/go/tools/cmd/staticcheck@v0.4.2
   ```
4. Linux build deps (if on Linux):
   ```bash
   sudo apt-get install gcc libgl1-mesa-dev libegl1-mesa-dev libgles2-mesa-dev libx11-dev xorg-dev
   ```
5. Verify you can run the full check locally:
   ```bash
   go test -tags ci ./...
   go vet -tags ci ./...
   goimports -e -d . | tee /dev/stderr   # must be empty
   gocyclo -over 30 .                    # must print nothing
   golint -set_exit_status $(go list -tags ci ./...)
   staticcheck -go 1.19 ./...
   ```
6. Build the reference app and sanity-check locally:
   ```bash
   go run ./cmd/fyneterm
   ```
7. Join `#fyne` on Gophers Slack (`http://gophers.slack.com/messages/fyne`) — this is where design questions actually get answered.

Branch per PR: `git checkout -b fix/modified-arrow-keys upstream/master` etc.

---

## 2. PR #1 — Fix Shift+arrow and add Alt/Ctrl+arrow sequences

**Lead with this one.** Smallest, clearest bug fix, no overlap with other in-flight PRs, exercises existing unused state.

### Why it's real

`input.go` already tracks `altPressed` and `ctrlPressed` in `keyboardState` (`term.go:86-90`, set in `trackKeyboardState` at `input.go:135-150`), but `TypedKey` only branches on `shiftPressed`. So:

- **Alt+Left/Right** → emit plain `\e[D` / `\e[C` (should be `\e[1;3D` / `\e[1;3C` per xterm, which readline/zsh then binds to `backward-word`/`forward-word`).
- **Ctrl+Left/Right** → same problem, should be `\e[1;5D` / `\e[1;5C`.
- **Shift+arrows** are emitted malformed. `keyTypedWithShift` writes `\e[A;2` at `input.go:124-131`; the xterm-compliant form is `\e[1;2A`. The existing tests in `input_test.go:63-66` **encode the bug**, which is why nobody has noticed — the fix requires updating the test table.

Cross-reference: no issue exists for this. Not obvious because most users don't notice word-motion is silently broken.

### Design decision: modifier codes

xterm / PuTTY / iTerm2 convention ([xterm ctlseqs](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html#h3-PC-Style-Function-Keys)):

```
\e[1 ; <mod> <final>    where <final> is A|B|C|D for Up|Down|Right|Left
                          and <mod> is:
                            2 = Shift
                            3 = Alt
                            4 = Shift+Alt
                            5 = Ctrl
                            6 = Shift+Ctrl
                            7 = Alt+Ctrl
                            8 = Shift+Alt+Ctrl
```

Implement the full matrix. It's no harder than just Alt, and Ctrl+arrow is bound by many users too.

### File-by-file changes

**`input.go`**

Replace the current dispatch in `TypedKey` (`input.go:23-84`) and `keyTypedWithShift` (`input.go:86-133`). Target shape:

```go
// TypedKey will be called if a non-printable keyboard event occurs
func (t *Terminal) TypedKey(e *fyne.KeyEvent) {
    lastKeyTime = time.Now()

    // Arrow and motion keys that honor all three modifiers.
    switch e.Name {
    case fyne.KeyUp, fyne.KeyDown, fyne.KeyLeft, fyne.KeyRight:
        t.typeCursorKey(e.Name)
        return
    case fyne.KeyHome, fyne.KeyEnd:
        t.typeMotionKey(e.Name)
        return
    }

    if t.keyboardState.shiftPressed {
        t.keyTypedWithShift(e)
        return
    }

    // (rest unchanged — F-keys, Enter, Tab, Escape, Backspace, Delete,
    // PageUp/Down, Insert)
    ...
}

// modifierCode returns the xterm modifier parameter (0 if no modifier).
func (t *Terminal) modifierCode() byte {
    var m byte = 1
    if t.keyboardState.shiftPressed { m += 1 }
    if t.keyboardState.altPressed   { m += 2 }
    if t.keyboardState.ctrlPressed  { m += 4 }
    return m
}

func (t *Terminal) typeCursorKey(key fyne.KeyName) {
    var final byte
    switch key {
    case fyne.KeyUp:    final = 'A'
    case fyne.KeyDown:  final = 'B'
    case fyne.KeyRight: final = 'C'
    case fyne.KeyLeft:  final = 'D'
    default:
        return
    }

    // No modifier + application-cursor mode: \eOA etc.
    // No modifier + normal mode:            \e[A etc.
    // With modifier (any combination):      \e[1;<m><final>
    mod := t.modifierCode()
    if mod == 1 {
        prefix := byte('[')
        if t.bufferMode {
            prefix = 'O'
        }
        _, _ = t.in.Write([]byte{asciiEscape, prefix, final})
        return
    }
    _, _ = t.in.Write([]byte{asciiEscape, '[', '1', ';', '0' + mod, final})
}

// typeMotionKey handles Home/End with the same modifier matrix.
// Home final is 'H' in CSI form ('\e[1;<m>H'), 'H'/'F' after 'O' in
// application mode.
func (t *Terminal) typeMotionKey(key fyne.KeyName) {
    final := byte('H')
    if key == fyne.KeyEnd {
        final = 'F'
    }
    mod := t.modifierCode()
    if mod == 1 {
        _, _ = t.in.Write([]byte{asciiEscape, 'O', final})
        return
    }
    _, _ = t.in.Write([]byte{asciiEscape, '[', '1', ';', '0' + mod, final})
}
```

**Delete** the separate arrow-key cases from `keyTypedWithShift` (`input.go:124-131`). Shift+arrow now flows through `typeCursorKey` which emits the correct `\e[1;2A` form. The rest of `keyTypedWithShift` (F-keys, Page/Home/End, Insert, Delete) is unaffected — or fold Home/End into the new path too.

Also consider bundling **Alt+Backspace** (`\e\x7f` — kill-word backward, universal zsh/bash binding) in `TypedKey`'s Backspace case:

```go
case fyne.KeyBackspace:
    if t.keyboardState.altPressed {
        _, _ = t.in.Write([]byte{asciiEscape, 0x7f})
    } else {
        _, _ = t.in.Write([]byte{asciiBackspace})
    }
```

**`input_test.go`**

The existing Shift+{Up,Down,Left,Right} test entries at `input_test.go:63-66` have the WRONG expected bytes (they encode the bug). Update to:

```go
"Shift+Up":    {fyne.KeyUp, false, true, []byte{asciiEscape, '[', '1', ';', '2', 'A'}},
"Shift+Down":  {fyne.KeyDown, false, true, []byte{asciiEscape, '[', '1', ';', '2', 'B'}},
"Shift+Left":  {fyne.KeyLeft, false, true, []byte{asciiEscape, '[', '1', ';', '2', 'D'}},
"Shift+Right": {fyne.KeyRight, false, true, []byte{asciiEscape, '[', '1', ';', '2', 'C'}},
```

Add new cases covering Alt/Ctrl/combinations. The existing table signature only has `shiftPressed bool`, so extend to:

```go
tests := map[string]struct {
    key          fyne.KeyName
    bufferMode   bool
    shiftPressed bool
    altPressed   bool
    ctrlPressed  bool
    want         []byte
}{
    // existing cases: add altPressed:false, ctrlPressed:false everywhere.
    // new cases:
    "Alt+Left":       {fyne.KeyLeft, false, false, true, false, []byte{asciiEscape, '[', '1', ';', '3', 'D'}},
    "Alt+Right":      {fyne.KeyRight, false, false, true, false, []byte{asciiEscape, '[', '1', ';', '3', 'C'}},
    "Ctrl+Left":      {fyne.KeyLeft, false, false, false, true, []byte{asciiEscape, '[', '1', ';', '5', 'D'}},
    "Ctrl+Right":     {fyne.KeyRight, false, false, false, true, []byte{asciiEscape, '[', '1', ';', '5', 'C'}},
    "Shift+Alt+Left": {fyne.KeyLeft, false, true, true, false, []byte{asciiEscape, '[', '1', ';', '4', 'D'}},
    "Shift+Home":     {fyne.KeyHome, false, true, false, false, []byte{asciiEscape, '[', '1', ';', '2', 'H'}},
    "Ctrl+Home":      {fyne.KeyHome, false, false, false, true, []byte{asciiEscape, '[', '1', ';', '5', 'H'}},
    "Home":           {fyne.KeyHome, false, false, false, false, []byte{asciiEscape, 'O', 'H'}},
    // if bundling Alt+Backspace:
    "Alt+Backspace":  {fyne.KeyBackspace, false, false, true, false, []byte{asciiEscape, 0x7f}},
}

// in the subtest setup:
term.keyboardState.shiftPressed = tt.shiftPressed
term.keyboardState.altPressed = tt.altPressed
term.keyboardState.ctrlPressed = tt.ctrlPressed
```

The existing `TestTerminal_TypedShortcut` cases ("LeftOption+U" etc.) continue unchanged — those go through a different code path (`TypedShortcut`, not `TypedKey`) and aren't affected.

### Coverage impact
Only adds tests; cannot drop coverage below 13%.

### Gocyclo check
`modifierCode` and the split helpers keep each function small. Current `TypedKey` is ~20 LOC; after split it stays well under complexity 30.

### Commit hygiene
Three commits for reviewability:
1. `input: route Home/End and arrow keys through a modifier-aware helper` (no behavior change for existing tests; preparatory refactor)
2. `input: emit xterm-compliant modifier codes for arrow and Home/End` (the bug fix; update existing Shift+arrow tests in the same commit since they encode the bug)
3. `input: add Alt/Ctrl/combination tests; handle Alt+Backspace` (new coverage)

Or one squashable commit if the maintainer prefers — check the merged history. Merges on this repo are mixed squash/merge.

### PR description template

```
Fix malformed Shift+arrow and add Alt/Ctrl+arrow (and combinations).

**What's broken today:**
- Shift+Up emits `\e[A;2` instead of xterm's `\e[1;2A`. The existing test
  table in input_test.go encodes the bug, so it has been passing against
  wrong bytes.
- Alt+arrow and Ctrl+arrow emit plain cursor keys — `keyboardState.alt-
  Pressed` and `ctrlPressed` are tracked in `trackKeyboardState` but never
  read in `TypedKey`. This breaks readline `backward-word`/`forward-word`
  and any `bindkey '\e[1;5D' ...` configuration.

**What this PR does:**
- Introduces `modifierCode()` returning the xterm parameter (1..8).
- Routes Up/Down/Left/Right and Home/End through `typeCursorKey` /
  `typeMotionKey`, emitting:
    - Unmodified normal:       `\e[<final>` (unchanged)
    - Unmodified app-cursor:   `\eO<final>` (unchanged)
    - Any modifier(s):         `\e[1;<mod><final>` (fixed / added)
- Fixes the four malformed Shift+arrow sequences.
- Adds Alt+Backspace → `\e DEL` (optional; kill-word-backward in readline).
- Updates existing tests and adds coverage for Alt/Ctrl/combinations.

No API surface change.

Tested locally: vim navigation, bash/zsh command-line editing with
`backward-word`/`forward-word` bindings.
```

---

## 3. PR #2 — Implement `fyne.Scrollable` (coordinate with PR #147)

### The situation

PR #147 "Add scroll support" by andydotxyz (7 files, +259/-59, OPEN since 2026-04-05) introduces real **scrollback** (`rowOffset()` plumbing, `clearScrollback`, etc.) — much bigger than "make mouse wheel do something." His PR does not obviously implement `fyne.Scrollable` on the widget; it builds the backing buffer.

Two viable plays:

### Option A — Wait for #147, then add `Scrollable` on top

If #147 merges, adding `fyne.Scrollable.Scrolled(ev)` becomes a natural follow-up:
- When there's scrollback above the viewport, wheel adjusts `rowOffset` and refreshes.
- When at the bottom (no scrollback to show), wheel emits cursor keys to the PTY (pager/readline behavior), matching what any serious terminal does.

Our biomelab `scrollForwarder` wrapper is exactly this "at the bottom" fallback. Contributing it upstream only makes sense as Phase 2.

### Option B — Tiny PR now: cursor-key forwarding only

If #147 is stalled, offer a standalone `fyne.Scrollable` impl that **only** forwards the wheel as cursor keys. No scrollback semantics. Totally compatible with #147 landing later (he can replace or augment). ~15 LOC:

```go
// term.go (new method)
func (t *Terminal) Scrolled(ev *fyne.ScrollEvent) {
    dy := ev.Scrolled.DY
    if dy == 0 { return }
    lines := int(dy / 12)
    if lines == 0 {
        if dy > 0 { lines = 1 } else { lines = -1 }
    }
    seq := []byte{asciiEscape, '[', 'A'}
    if lines < 0 {
        seq = []byte{asciiEscape, '[', 'B'}
        lines = -lines
    }
    for i := 0; i < lines; i++ {
        _, _ = t.in.Write(seq)
    }
}
```

**Before writing code**, comment on #147 asking Andy whether he'd accept a tiny `fyne.Scrollable` stub now (useful for pagers) or wants the whole thing folded into his scrollback PR. This is a Slack question, not a PR draft yet.

### Test
Add to `input_test.go` or a new `scroll_test.go`:

```go
func TestTerminal_Scrolled(t *testing.T) {
    tests := map[string]struct {
        dy   float32
        want []byte
    }{
        "ScrollUp_SmallDelta":   {12, []byte{asciiEscape, '[', 'A'}},
        "ScrollUp_LargeDelta":   {48, bytes.Repeat([]byte{asciiEscape, '[', 'A'}, 4)},
        "ScrollDown_SmallDelta": {-12, []byte{asciiEscape, '[', 'B'}},
        "Zero":                  {0, []byte{}},
    }
    for name, tt := range tests {
        t.Run(name, func(t *testing.T) {
            buf := bytes.NewBuffer([]byte{})
            term := &Terminal{in: NopCloser(buf)}
            term.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DY: tt.dy}})
            if !bytes.Equal(buf.Bytes(), tt.want) {
                t.Errorf("Scrolled() = %v, want %v", buf.Bytes(), tt.want)
            }
        })
    }
}
```

### biomelab payoff
Either option lets us delete `scrollForwarder` from `internal/gui/termpanel.go`.

---

## 4. PR #3 — Expose a resize hook for `RunWithConnection` (closes #6, #103)

### The situation
`updatePTYSize` in `term_unix.go:17-29` early-returns when `t.pty == nil` — which is always the case for `RunWithConnection`. So connection-mode callers can't forward resize.

Current workaround (ours and the one the README suggests):
```go
ch := make(chan terminal.Config)
t.AddListener(ch)
go func() {
    for cfg := range ch {
        session.WindowChange(int(cfg.Rows), int(cfg.Columns))
    }
}()
```

This works but has pitfalls: `Config` events also fire on PWD change, so consumers must dedup (we do); the channel must be drained; ordering between `AddListener` and `RunWithConnection` matters.

### Proposed API
Add an explicit callback set before `RunWithConnection`:

```go
// SetResizeHandler registers a callback invoked whenever the terminal
// widget is resized. Rows and cols are the post-resize terminal dimensions.
// Intended for RunWithConnection callers (e.g. SSH, custom PTY) so they
// can forward SIGWINCH to the remote side. Has no effect in RunLocalShell
// mode, which manages its own PTY resize.
func (t *Terminal) SetResizeHandler(fn func(rows, cols uint)) {
    t.resizeHandler = fn
}
```

In `term_unix.go` / `term_windows.go` `updatePTYSize`:

```go
func (t *Terminal) updatePTYSize() {
    if t.resizeHandler != nil {
        t.resizeHandler(t.config.Rows, t.config.Columns)
    }
    if t.pty == nil { return }
    // ...existing body unchanged
}
```

This matches the user request in #103 verbatim and is strictly additive — the existing `AddListener` path still works.

### Before coding
Comment on **#103** with the proposed API and tag @andydotxyz. ~2 days is reasonable to wait. If silence, ping in Slack. This is the right etiquette for a touching-public-API change.

### Tests
`term_test.go`:

```go
func TestTerminal_SetResizeHandler(t *testing.T) {
    term := terminal.New()
    var gotRows, gotCols uint
    term.SetResizeHandler(func(r, c uint) {
        gotRows, gotCols = r, c
    })
    term.config.Rows = 24
    term.config.Columns = 80
    term.updatePTYSize()  // will need to be exported or tested via Resize
    if gotRows != 24 || gotCols != 80 {
        t.Errorf("handler got %dx%d, want 24x80", gotRows, gotCols)
    }
}
```

(If `updatePTYSize` stays unexported, trigger via `term.Resize(size)` after forcing columns/rows through a fake canvas.)

### biomelab payoff
We delete the `AddListener` / `cfgCh` loop in `internal/gui/termpanel.go#runSandboxCommand` / `startSandbox` and replace with one line: `t.SetResizeHandler(func(r,c uint) { pty.Setsize(ptyFile, &pty.Winsize{Rows:uint16(r),Cols:uint16(c)}) })`.

---

## 5. Issue to file BEFORE coding — `guessCellSize` caching

**Do not open a PR first.** PR #136 (same topic, debounce approach) was CLOSED by the author with no review comments. That's an unexplained rejection; we need context.

Root cause: `term.go:247-268` `Resize` calls `guessCellSize()` every time, which constructs a `canvas.Text` and calls `MinSize()`. The code has a `// don't call often - should we cache?` comment at `term_unix.go:377`. On VSplit drags this pegs the UI thread (what biomelab users feel as a freeze).

### File an issue on fyne-io/terminal with this body:

```
## Resize perf: `guessCellSize()` recomputes text metrics on every pixel

**Current behavior:** `Terminal.Resize` (term.go:247) calls
`guessCellSize()` on every pixel-level resize event. `guessCellSize`
(term_unix.go:377) constructs a `canvas.Text` and calls `MinSize()`,
which in Fyne is a non-trivial text-measurement op. During a container
resize drag, this runs hundreds of times per second on the UI thread.

The source already acknowledges this: `// don't call often - should we
cache?` (term_unix.go:376).

**User-visible impact:** Terminal panel embedded in a resizable container
(e.g. a VSplit) freezes / stutters while the user is dragging the
divider. `systemctl status`, `nano`, `htop` amplify the stutter because
they trigger full redraws on every SIGWINCH.

**Proposed fix:** Cache the result of `guessCellSize` keyed on the
current theme text size. Invalidate via the existing theme-listener hook
(the widget already calls `theme.TextSize()` inside `guessCellSize`, so
we can pre-read it, cache, and invalidate when it changes).

This is adjacent to #78. PR #136 tried a related angle (debouncing Resize
itself) and was closed without review comments — could you share what
approach would be preferred?

Happy to PR if the caching shape is acceptable.
```

Wait for a response before implementing.

---

## 6. Issue / PR — fix Backspace `^H` vs `^?` (#138)

Orthogonal but tiny. In `input.go:67`:

```go
case fyne.KeyBackspace:
    _, _ = t.in.Write([]byte{asciiBackspace})   // 0x08 == ^H
```

`asciiBackspace = 8` (output.go:16). But `stty` on Linux/macOS says `erase = ^?` (0x7F). That's why `bash` / `zsh` show `^H` literally when you hit backspace inside `fyneterm`.

The issue reporter (#138) suggests "expose functions to override special key codes." Simpler fix: just change the default to `0x7F`. But this breaks anyone who configured their shell to use `^H` — which is the minority.

### Options to discuss
1. Change default, document.
2. Add a setter (`SetBackspaceCode(byte)`).
3. Auto-detect from `stty` at startup (complex, platform-dependent).

**Recommendation:** option 1 + comment in #138 proposing it. Don't bundle with PR #1 — keep that focused on arrow keys.

---

## 7. Execution order & timing

| Phase | Action | Duration | Blocking? |
|---|---|---|---|
| T0 | Fork, clone, set up tools, run CI locally | 30 min | — |
| T0 | Join `#fyne` Slack, skim recent messages | 15 min | — |
| T1 | **PR #1: modified arrow keys** — draft, self-review, open | 2–4 hrs | No |
| T1 | Comment on **#103** proposing `SetResizeHandler` API | 10 min | Wait 2 days |
| T1 | Comment on **#147** asking about Scrollable coexistence | 10 min | Wait 2 days |
| T1 | File **issue** on `guessCellSize` caching | 20 min | Wait 2 days |
| T1 | File **issue/comment on #138** for backspace | 15 min | — |
| T2 (after #103 response) | **PR #3: resize handler** | 2 hrs | No |
| T2 (after #147 clarity) | **PR #2: `fyne.Scrollable`** (option A or B) | 2 hrs | No |
| T3 (after caching issue response) | **PR: guessCellSize caching** | 4 hrs | No |
| T4 (after all merge) | biomelab: bump dep, delete `scrollForwarder`, delete `registerTerminalKeyShortcuts`, delete resize listener goroutine | 30 min | — |

---

## 8. Appendix — biomelab code that goes away when each PR merges

| Upstream lands | Delete from biomelab |
|---|---|
| PR #1 (arrow keys) | `internal/gui/keycapture.go` → `registerTerminalKeyShortcuts` entirely, plus its call site in `internal/gui/app.go` |
| PR #2 (`fyne.Scrollable`) | `internal/gui/termpanel.go` → `scrollForwarder` type, its uses in `newSession`, revert `view` field back to bare `*terminal.Terminal` |
| PR #3 (resize handler) | `internal/gui/termpanel.go` → the `AddListener` goroutine in `startSandbox`, replaced by one-line `t.SetResizeHandler(...)` |
| Issue #138 fix | No biomelab-side cleanup (we never worked around it) |
| Caching PR | No biomelab-side cleanup; just the VSplit freeze goes away |

Keep this document updated as PRs move through review.
