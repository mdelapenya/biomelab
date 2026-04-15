# Testing TUIs with Confidence: A Design Document

## Problem Statement

Terminal User Interfaces (TUIs) built with frameworks like Bubbletea combine
user input handling, state management, and terminal rendering in a single loop.
This creates testing challenges:

1. **State + rendering coupling** — The Update function mutates state while View
   reads it. Testing one without the other gives false confidence.
2. **Async commands** — Bubbletea commands (`tea.Cmd`) run in goroutines. Tests
   must handle timing without flakiness.
3. **Terminal output** — View output contains ANSI escape codes from lipgloss
   styling, making string comparison fragile.
4. **External dependencies** — TUIs often shell out to system commands (git, ps)
   that must be isolated in tests.

## Testing Pyramid for TUIs

```
                    ┌──────────┐
                    │ Full-loop│  ← Few: real program with mock I/O
                   ╱│ Integr.  │╲
                  ╱ └──────────┘ ╲
                 ╱  ┌──────────┐  ╲
                ╱   │  Golden  │   ╲ ← Medium: snapshot View() output
               ╱    │  Files   │    ╲
              ╱     └──────────┘     ╲
             ╱      ┌──────────┐      ╲
            ╱       │  Model   │       ╲ ← Many: pure Update() state tests
           ╱        │  Unit    │        ╲
          ╱         └──────────┘         ╲
         ╱          ┌──────────┐          ╲
        ╱           │ Domain   │           ╲ ← Foundation: git, agent, etc.
       ╱            │ Logic    │            ╲
      ╱─────────────└──────────┘─────────────╲
```

## Layer 1: Domain Logic Tests

### Strategy
Test your business logic packages (git operations, agent detection) in complete
isolation from the TUI. These form the foundation of confidence.

### Patterns

**Interface-based command abstraction:**
```go
type CommandRunner interface {
    Output(name string, args ...string) ([]byte, error)
}
```
Every package that calls external commands accepts this interface. Tests provide
a mock that returns canned output.

**Real temporary repos for git tests:**
```go
func setupTestRepo(t *testing.T) (string, *gogit.Repository) {
    dir := t.TempDir()  // auto-cleaned
    repo, _ := gogit.PlainInit(dir, false)
    // create initial commit...
    return dir, repo
}
```
Using real git repos (not mocks) for git-related tests gives true confidence
that your parsing and operations work. `t.TempDir()` handles cleanup.

**Table-driven parsing tests:**
```go
func TestParseLsofOutput(t *testing.T) {
    tests := []struct {
        name string
        data string
        want map[string]string
    }{
        {"two procs", "p123\nn/path\np456\nn/other\n", map[string]string{"123": "/path", "456": "/other"}},
        {"empty", "", map[string]string{}},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := parseLsofOutput([]byte(tt.data))
            // assert...
        })
    }
}
```

## Layer 2: Model Unit Tests (Update Function)

### Strategy
The `Update` function is a pure state machine: `(Model, Msg) -> (Model, Cmd)`.
Test it by sending synthetic messages and asserting the resulting model state.

### Patterns

**Synthetic message injection:**
```go
func TestNavigateRight(t *testing.T) {
    m := testModel(3)  // 3 worktrees
    m.cursor = 0

    updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
    model := updated.(Model)

    assert(model.cursor == 1)
}
```

**Test helper that bypasses Init:**
```go
func testModel(n int) Model {
    ti := textinput.New()
    m := Model{
        keys:      defaultKeyMap(),
        agents:    make(DetectionResult),
        textInput: ti,
    }
    for i := range n {
        m.worktrees = append(m.worktrees, Worktree{...})
    }
    return m
}
```
This creates a model with pre-populated state, skipping `Init()` which would
start async commands. Critical for deterministic tests.

**Command inspection:**
When `Update` returns a `tea.Cmd`, you can execute it synchronously to get
the resulting message:
```go
updated, cmd := m.Update(someMsg)
if cmd != nil {
    resultMsg := cmd()  // execute synchronously
    // assert resultMsg is what you expect
}
```

**Modal state transitions:**
Test every state transition path, especially mode changes (normal → create →
normal, normal → confirm delete → normal):
```go
func TestDeleteMainBlocked(t *testing.T) {
    m := testModel(2)
    m.cursor = 0  // main worktree

    updated, _ := m.Update(tea.KeyMsg{Runes: []rune{'d'}})
    model := updated.(Model)

    assert(model.mode == modeNormal)  // should NOT enter confirm mode
    assert(model.statusMsg != "")     // should show error
}
```

### What to Assert
- `model.cursor` position after navigation
- `model.mode` after mode-switching keys
- `model.worktrees` after `refreshMsg`
- `model.err` and `model.statusMsg` after error conditions
- That cursor is clamped when worktrees shrink

### What NOT to Assert
- Specific View() output in Update tests (separate concern)
- That `tea.Cmd` functions do the right thing (test the message handlers instead)

## Layer 3: View/Golden File Tests

### Strategy
Capture the output of `View()` and compare against checked-in golden files.
This catches unintended rendering regressions without asserting exact strings
in code.

### Pattern: Golden Files with Update Flag
```go
var update = flag.Bool("update", false, "update golden files")

func TestView_MultipleCards(t *testing.T) {
    m := testModel(3)
    m.width = 120
    m.height = 40

    got := m.View()

    golden := filepath.Join("testdata", "golden", "view_multiple.txt")
    if *update {
        os.WriteFile(golden, []byte(got), 0o644)
    }
    want, _ := os.ReadFile(golden)
    if diff := cmp.Diff(string(want), got); diff != "" {
        t.Errorf("View mismatch (-want +got):\n%s", diff)
    }
}
```

Run `go test -update` to regenerate golden files when you intentionally change
the UI. In CI, golden files are read-only assertions.

### Dealing with ANSI Codes
Two approaches:
1. **Include ANSI in golden files** — catches style regressions, but diffs are
   hard to read. Works well with `cmp.Diff`.
2. **Strip ANSI before comparison** — easier to read, misses style bugs.
   Use a regex like `\x1b\[[0-9;]*m` to strip.

Recommendation: **include ANSI** for card-level golden tests (where styling IS
the feature), strip for high-level layout tests.

### Fixed Dimensions
Always set `m.width` and `m.height` to fixed values in golden tests. Terminal
dimensions vary between machines and CI; non-deterministic widths break golden
files.

## Layer 4: Race Condition Tests

### Why This Matters
Bubbletea calls `View()` from a separate goroutine for rendering while
`Update()` runs in the main goroutine. If your model mutates slices or maps
in `Update` that `View` reads, you have a data race.

### Pattern: Concurrent View + Update
```go
func TestConcurrentViewAndUpdate(t *testing.T) {
    m := testModel(5)
    var wg sync.WaitGroup

    // Readers: call View() concurrently.
    wg.Add(1)
    go func() {
        defer wg.Done()
        for range 1000 {
            _ = m.View()
        }
    }()

    // Writers: call Update() concurrently.
    wg.Add(1)
    go func() {
        defer wg.Done()
        for range 1000 {
            m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
        }
    }()

    wg.Wait()
}
```

Run with `go test -race`. The race detector will catch unsynchronized access.

### The Right Fix
If your model follows Bubbletea's design correctly:
- `Update()` is the only writer (called sequentially by the framework)
- `View()` is a pure reader (no mutations)

Then **no mutexes are needed**. The race test validates this invariant. If
it fails, you likely have a mutation in `View()` — fix that rather than
adding locks.

## Layer 5: Full-Loop Integration Tests

### Strategy
Spin up the real TUI with controlled I/O, send keystrokes, and assert output.
Use sparingly — these are slow and brittle.

### Pattern: Bubbletea Test Mode
```go
func TestFullLoop(t *testing.T) {
    in := bytes.NewReader([]byte("q"))  // simulate pressing 'q'
    var out bytes.Buffer

    m := New(repo, detector)
    p := tea.NewProgram(m,
        tea.WithInput(in),
        tea.WithOutput(&out),
    )
    _, err := p.Run()
    assert(err == nil)
    assert(strings.Contains(out.String(), "biomelab"))
}
```

### When to Use
- Smoke test that the program starts and quits cleanly
- Testing that Init → first refresh → View produces expected output
- Verifying key combinations trigger expected screen changes

### When NOT to Use
- Testing individual state transitions (use Model unit tests)
- Testing rendering details (use golden files)
- Testing business logic (use domain tests)

## Anti-Patterns to Avoid

1. **Testing via screenshots** — Taking terminal screenshots and comparing
   pixels. Fragile across font sizes, terminal emulators, and OS versions.

2. **Sleep-based timing** — `time.Sleep(100*time.Millisecond)` in tests to
   wait for async operations. Use channels or synchronous command execution.

3. **Testing private methods** — If you need to test internal logic, it
   probably belongs in a separate package (like `card.Render()` being a public
   function in its own package).

4. **Mocking Bubbletea itself** — Don't mock `tea.Program` or `tea.Model`.
   Test your Model directly since it's just a state machine.

5. **Single giant integration test** — One test that sends 50 keystrokes and
   checks final output. Break it into focused unit tests.

## Test Matrix Summary

| What to Test            | How                          | Speed  | Confidence |
|------------------------|------------------------------|--------|------------|
| Git operations         | Real temp repos              | Medium | High       |
| Process detection      | Mock CommandRunner           | Fast   | High       |
| State transitions      | Synthetic messages to Update | Fast   | High       |
| Card rendering         | Golden files                 | Fast   | Medium     |
| Full-screen layout     | Golden files with fixed dims | Fast   | Medium     |
| Concurrency safety     | Race detector + goroutines   | Fast   | High       |
| Startup/shutdown       | Full-loop with mock I/O      | Slow   | Medium     |

## CI Configuration

```makefile
test:
	go test -race -v ./...

test-update-golden:
	go test -v -update ./internal/tui/... ./internal/tui/card/...

lint:
	golangci-lint run ./...
```

Run `make test` in CI. Run `make test-update-golden` locally after intentional
UI changes, then commit the updated golden files.
