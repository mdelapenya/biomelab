# Biomelab terminal compatibility patch

This directory is a source snapshot of `github.com/fyne-io/terminal` at
`c8f30fa130e35b342233cf2ee3171d3a161039bd` (2026-09-27), with its LICENSE and
upstream tests retained. The root module uses an explicit local `replace` so
builds do not depend on an unpublished fork or mutate the Go module cache.
Only the library, tests, and test fixtures are included; the upstream executable
and promotional assets are omitted.

The scrollback implementation is adapted from
[fyne-io/terminal PR 147](https://github.com/fyne-io/terminal/pull/147), head
`0de34d0c3536a94bb32ce85d88fdfcf42716214b`, retrieved 2026-10-07. It was rebased
onto the snapshot above, preserving newer alternate-screen and escape handling.
This is a local compatibility patch, not an upstream release.

## Local changes

- Bounded scrollback (1,000 lines), scroll-container rendering, and screen versus
  history coordinates, adapted from PR 147. Alternate-screen programs do not
  append their screen contents to normal shell history.
- Correct modified cursor sequences through TypedKey and TypedShortcut,
  Shift+Tab, DEL backspace, and an explicit Ctrl+Shift+Space callback.
- Parse each output batch and refresh its cursor together on the UI thread.
  Per-character queued updates previously left the native cursor one batch
  behind; the software test driver hid this by running callbacks inline.
- Retain incomplete UTF-8 across reads instead of dropping split characters.
- Optional indexed-color palette hook, used by Biomelab for readable ANSI colors
  in both themes, including retained history. Explicit RGB colors stay unchanged.
- `AttachWriter`, `Feed`, `Dimensions`, and `OnResize` provide an app-owned
  transport integration. Biomelab calls these on the Fyne thread; it does not
  call RunLocalShell or RunWithConnection. Output parsing and canvas mutation
  therefore happen on the same thread, and resize delivery is direct.
- Preserve viewport geometry and cursor bounds while resizing. Ignore transient
  zero-cell layouts when rebuilding the workspace, retaining the last valid
  PTY dimensions.
- `Feed` preserves the scroll position when reading earlier output and follows
  the bottom when already there. It sizes the scroll content to the new history
  before scrolling, because the Fyne scroller clamps against the content's
  laid-out size; without that, a burst of rows arriving in one read (typical
  of ConPTY) left the viewport pinned at the top of the scrollback. The scroll
  wheel never sends unconditional cursor-key input into the shell.
- String sequences end correctly with ST (ESC followed by a backslash).
  Upstream only completed OSC that way: an APC ended with ST never closed and
  swallowed all later output, a DCS ended with ST ate the next printable
  character, and a bare backslash inside DCS payload ended it early. Any
  other ESC aborts the sequence in progress (string, partial CSI or charset
  selection), as in xterm, so stale parameters never leak into the next one.
  OSC, DCS and APC payload is capped at 8 KiB so unterminated or huge
  sequences cannot grow an unbounded string on the UI thread.
- Escape handling treats process output as untrusted. CSI final characters
  are split as runes (a multibyte final used to panic), delete/insert
  character and SM/RM/DA handlers are bounds-checked, every numeric CSI
  parameter is clamped to 9999, the scroll region is clamped to the screen,
  and a recover around each escape handler limits any missed panic to that
  one sequence instead of the application.
- Copy with no selection leaves the clipboard untouched (upstream copied the
  terminal's first character).
- Erase and cursor semantics follow ECMA-48/xterm: ED 1 and EL 1 erase
  through the cursor cell (inclusive) with spaces in the current colours,
  and ED 1 clears every row above the cursor; CUP with only a row parameter
  goes to that row, column 1; SU scrolls the region wherever the cursor is,
  never clears outside the region, scrolls a buffer still shorter than the
  screen, and leaves the cursor in place. The upstream
  `TestScrollBack_With_Zero_Back_Buffer` expected SU to move the cursor and
  is updated accordingly.
- Double-clicking the last character of a row selects its word (the 1-based
  column bound was off by one).
- DL and IL act only at and below the cursor inside the scroll region and
  delete or insert at most the lines left there (upstream mutated the region
  from outside it, and IL with a count above one copied rows from above the
  cursor). SGR 27 swaps back what SGR 7 swapped, restoring default colours,
  instead of clearing both colours; SGR 0 ends reverse video.
- The blink ticker is stopped when the renderer that created its grid is
  destroyed, instead of leaking one goroutine per restarted session.
- OSC 7 (shell-reported working directory) is parsed by hand and stored in
  `Config.PWD` for listeners. Upstream called `os.Chdir` on the host process,
  which let any shell output move Biomelab's own working directory (and on
  Windows lock the worktree), and its malformed-URI fallback could index past
  short input. Unencoded `?`, `#` and `%` stay part of the path, reports from
  another host are ignored, and the drive-letter form is only rewritten on
  Windows.
- The upstream exit-code tests that spawn `RunLocalShell` are skipped on
  Windows: the retained ActiveState ConPTY path never reports an exit code
  there, so the test would spin until the package timeout. Biomelab's Windows
  transport lives in `internal/embeddedterminal` and has its own tests.
- The module's minimum Go version and Fyne requirement match the Fyne 2.8 line.

The application owns process cancellation, input queuing, PTY cleanup, and
process reaping in `internal/embeddedterminal`. The upstream Run APIs are retained
for provenance and upstream tests; their concurrency behavior is not the
application's integration contract.

## Validation and upstream candidates

Run from the Biomelab repository root so its dependency selection is used:

```sh
go test github.com/fyne-io/terminal/...
go test -race github.com/fyne-io/terminal/... -run TestEmbedded
```

`embedded_test.go` covers the focused shortcut path, Tab/Escape ownership,
alternate-screen history restoration, history bounds, split UTF-8, and resizing.
`render_test.go` checks font sizes and uses a queued driver to verify that
output and cursor state are current when Feed returns. The updated existing input tests expect valid Shift+arrow sequences and DEL.
The root Taskfile and CI run compatibility tests explicitly because `./...`
does not traverse nested modules.

Prepare separate upstream contributions for modified-key encoding and split
UTF-8 handling. Coordinate the scrollback fixes with PR 147. The Feed/resize API
needs maintainer discussion before proposing a public API change. Before each
contribution, reproduce against the then-current upstream head, check for
existing fixes, and separate application-specific keybindings from library fixes.
A broader race run also reports races in the retained `TestTerminal_Close`
(`RunLocalShell` startup versus `Write`/`Close`). This is outside Biomelab's
app-owned transport path; reproduce against unmodified upstream before filing.
The integration race checks above deliberately target the embedded API.
No contribution has been published from this worktree.

When upstream covers these needs, remove this replacement and snapshot only
after the same integration tests pass with the selected upstream version.
