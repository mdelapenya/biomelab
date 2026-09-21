# Windows focus regression verification

Issue [#81](https://github.com/mdelapenya/biomelab/issues/81) reports constant
keyboard focus loss and briefly appearing terminal windows. The fixes suppress
background command consoles and bound refresh/action work. A passing build or
absence of visible flashes does not prove uninterrupted keyboard focus.

## Capture the reported failure

Use an interactive Windows desktop and the packaged executable launched from
Explorer. Record the Windows version, Biomelab version/commit, regular/sandbox
mode, repository/worktree count and relevant environment settings. Repeat with
the same configuration before and after the fix.

Build the optional foreground observer from the repository on Windows:

```powershell
go build -o focus-trace.exe ./cmd/helpers/focus-trace
.\focus-trace.exe -duration 5m -out before-focus.csv
```

The observer uses
[SetWinEventHook](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setwineventhook)
to record foreground transitions rather than polling, so brief transitions can
be captured. It writes observation time, event uptime, HWND and PID, without
window titles, typed text or command lines. It creates a new file and refuses
to overwrite one. It does not activate windows. The initial row has event
uptime zero; subsequent uptime values are Windows event timestamps.

Run Microsoft's
[Process Monitor](https://learn.microsoft.com/en-us/sysinternals/downloads/procmon)
alongside it to correlate process creation/exit and Biomelab's descendant tree
with the focus trace. A window that closes before PID resolution may have PID
zero; use the surrounding process events and recording to investigate it.
Process Monitor captures more information than this observer; inspect a trace
before sharing it.

## Scenarios

For at least five minutes each in representative regular and sandbox modes:

1. Keep typing in another application's editor while Biomelab is visible in the
   background. Observe startup and multiple 5-second local / 30-second default
   network refresh cycles.
2. Repeat with Biomelab minimized, then hidden to the system tray.
3. Deliberately return to Biomelab, switch repos/modes and trigger refresh bursts,
   then return to the editor. Note those intentional focus changes in the record.
4. Include unavailable or unauthenticated CLIs and an unavailable sandbox daemon.
   Authentication should be completed separately in a terminal; unattended Git
   credential lookup disables Git terminal and Git Credential Manager prompts.
5. Press Enter repeatedly on a card. On Windows, the current terminal launcher
   is unsupported and should display an error without launching a console,
   including when `BIOME_TERMINAL` is set. Open terminals manually. On supported
   macOS/Linux setups, verify that one launch stays usable and reuse still works.
6. Verify intentional editor, system-file and native save-dialog actions remain
   visible and functional, without an extra PowerShell console for Save.

Repeat the observer with `-out after-focus.csv`. Pass only if typing remains
uninterrupted and there are no unsolicited foreground transitions to Biomelab
or its descendants, whether or not consoles flash. Record the build SHA,
configuration, scenario and before/after result. A confirmed residual GUI focus
defect needs investigation even if command suppression works.

## Automated coverage and limits

Windows CI executes the command runtime tests through a GUI-subsystem launcher,
checks that background children have no console window, and tests Windows
terminal rejection before launch. Cross-platform tests cover command I/O,
errors, cancellation, inherited output-pipe waits, refresh coalescing and stale
results, and terminal request/error handling.

`CREATE_NO_WINDOW` applies to each configured command; arbitrary third-party
descendants can still create consoles or dialogs. The integration descendant
test applies the policy explicitly at each generation. Cancelling a command
kills its direct process and bounds waits for inherited pipes; it does not
guarantee termination of every descendant. Check configured credential helpers
and sandbox tools in the desktop trace. Automated/headless tests and the observer's
cross-compilation do not constitute a completed desktop acceptance run.
