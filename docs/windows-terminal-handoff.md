# Windows integrated terminal review

This handoff is for native Windows validation of the integrated card terminal.
The implementation combines the integrated-terminal baseline with the Windows
ConPTY work; neither branch has been published yet. Review the combined draft
branch against its recorded base. Windows CI already runs on `windows-latest`
and checks the app-owned transport, but native desktop behavior still needs a
Windows reviewer.

## Prerequisites and commands

Use Windows 10 version 1809 or later, or Windows 11. ConPTY is required; when
the API is unavailable, opening the integrated terminal reports the startup
error and **Open in external terminal** is an explicit alternative. Review the
existing [Windows build and focus instructions](windows-focus-validation.md)
for the Go toolchain, desktop build, and CI context. Run these commands from the
repository root in PowerShell:

```powershell
go test -race -count=1 ./internal/embeddedterminal
go test -race -count=1 ./internal/gui
go test github.com/fyne-io/terminal/...
go test -race github.com/fyne-io/terminal/... -run TestEmbedded
go build -o bin/biomelab.exe ./cmd/biomelab
```

The first two commands exercise Biomelab's app-owned ConPTY lifecycle and GUI
integration on Windows. The terminal widget commands check the local
compatibility patch. Its retained upstream `RunLocalShell` tests have produced
known race reports; keep any such report separate from Biomelab's backend or GUI
failures, and include the exact command and output. Do not classify an app-owned
failure as an upstream widget race. The backend suite covers argument and
working-directory handling, resize/exit, natural-exit output and EOF, canceled
startup, descendant cleanup, blocked reads, and large output. The GUI suite
includes `TestCardTerminalWindowsFocusesAndReusesIntegratedSession`,
`TestCardTerminalWindowsRealShellLifecycle`, and
`TestCardTerminalWindowsStartFailureStaysInDrawer`. The backend also includes
`TestConPTYRootExitWithDescendantClosesSession`, which checks root-shell exit
with a long-lived owned descendant.

## Interactive desktop checklist

Run the built application on a real Windows desktop. Record Windows edition,
version/build, Biomelab revision, Go version, shell executable/version, and the
exact command and output for each failure. The native CI tests do not establish
visual layout, focus, clipboard, or desktop acceptance.

## Known baseline caveat

Transient oversized or busy-paste feedback can remain in the session status
until the next lifecycle event. Treat that as a pre-existing terminal UI status
issue, not evidence that the Windows process stopped.

1. Press Enter on a linked worktree, then use Main's **Terminal** action and
   the inspector's **Open Terminal** action. Each opens the drawer without
   creating a separate OS terminal window. Confirm the default shell resolves
   in this order: `pwsh.exe -NoLogo`, `powershell.exe -NoLogo`, then
   `%COMSPEC%` or `cmd.exe`.
2. Test a worktree path containing spaces and non-ASCII characters. Check the
   initial working directory, command arguments, typing, cursor movement,
   resize, and scrollback under sustained and large output.
3. Check Tab, Escape, Ctrl+C, copy, and paste. Ctrl+Shift+Space should return
   focus to workspace navigation. Confirm clipboard shortcuts work in the
   terminal and do not swallow ordinary shell input.
4. Switch between cards and back. Existing sessions should stay alive and
   return without duplicates. Hide and reopen the drawer; a running session
   should continue. Type `exit` in the shown shell and confirm the drawer
   collapses and workspace focus returns. Open that card again and confirm a
   fresh session starts. An exit in another card must leave the shown terminal
   open; abnormal exits and startup failures must stay visible for diagnosis.
   Start a long-lived child process from the root shell, then exit the root
   shell. Confirm the panel closes only after final output drains and all owned
   descendants are terminated.
   In List view, expand a linked card's terminal. The pinned card above it must
   show that card's task, branch, path, repository, and mode. Switch cards while
   expanded, then restore the drawer; the owner context and Main card must
   follow those changes.
5. Use **Stop session** and **Restart session**. Confirm the process tree ends
   before a restarted shell becomes active. Quit Biomelab and confirm owned
   descendants stop.
6. With two sessions open, delete a linked worktree that owns one session.
   Confirm deletion waits for that session's cleanup and leaves the unrelated
   session running. If cleanup cannot complete within the bounded wait, expect
   an error and no worktree removal. Reopening that worktree's terminal or
   retrying deletion must remain blocked until cleanup is confirmed; then
   retry deletion. The unrelated session must remain usable throughout.
7. If `sbx` is available, check the sandbox main card and a linked worktree
   card. Confirm the shell/agent starts in the expected container directory.
8. Choose **Open in external terminal** from the terminal menu. Confirm this
   deliberate action still opens or reuses the configured external terminal.
9. Repeat open, switch, stop, restart, and close with repeated sessions and
   large output. Record hangs, duplicate processes, incorrect directories,
   missing output, or cleanup delays with the exact scenario.

Windows CI provides native backend and GUI test execution through
[`windows-latest`](../.github/workflows/ci.yml); this checklist provides desktop
visual and interaction evidence. Report those results separately.
