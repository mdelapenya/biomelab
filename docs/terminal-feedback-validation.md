# Integrated terminal desktop feedback

Follow-up to the [Windows desktop report](https://github.com/mdelapenya/biomelab/pull/95#issuecomment-6078434988).
The external-launch timeout is already addressed by #96, which is in this branch's base.

## Changes

- Output parsing defers per-cell and line-scroll rendering until the end of each
  PTY chunk, keeping the existing synchronous UI dispatch and bounded read buffer.
- Clearing scrollback shrinks the scroll content and clamps the viewport, both
  while following output and while reading earlier history.
- The Enter event that focuses the terminal is consumed by identity; subsequent
  Enter presses are delivered normally (including keypad Enter).
- Removing the shown session collapses the drawer before selection changes, so
  another card's retained exited session does not unexpectedly appear.
- Expand/Restore releases focus before rebuilding and restores it afterward.
  Restart attaches the replacement view before requesting focus.
- The tray Quit item explicitly sets Fyne's `IsQuit`, preventing an additional
  localized Quit item while preserving application cleanup.

## Local performance measurement

`BenchmarkEmbeddedOutput2000Lines` feeds 2,000 70-byte lines in 32 KiB chunks
through the embedded API with an 800 × 500 terminal. On the same macOS Intel
machine and Fyne test driver, one iteration took 14.66 s before these changes
and 0.20 s afterward (approximately 73× faster). This isolates emulator/render
work; it does not measure ConPTY or native Windows frame/input latency.

Run from the repository root:

```sh
go test -race ./...
go test github.com/fyne-io/terminal/...
go test -race github.com/fyne-io/terminal/... -run TestEmbedded
go test github.com/fyne-io/terminal -run '^$' -bench BenchmarkEmbeddedOutput2000Lines -benchtime=1x
```

## Windows desktop checks still required

Repeat the original Windows 11 / PowerShell / 125% scaling scenario: 2,000 lines
with typing during output, `cls` after scrolling and in expanded mode, opening
with Enter, deleting the shown worktree while another card has exited, repeated
Expand/Restore and Restart, and the tray menu in a non-English locale. Native
Windows behavior and sandbox-card sessions were not exercised on the local
macOS host.
