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

For at least five minutes each in representative regular and sandbox modes,
perform the background/focus scenarios below and the terminal acceptance
checklist. This is a laptop-desktop procedure: use a host worktree with spaces
and non-ASCII characters in its path, and exercise it through quoted
PowerShell arguments (Windows file names themselves cannot contain `"`). Record
the exact paths and terminal versions used.

1. Keep typing in another application's editor while Biomelab is visible in the
   background. Observe startup and multiple 5-second local / 30-second default
   network refresh cycles.
2. Repeat with Biomelab minimized, then hidden to the system tray.
3. Deliberately return to Biomelab, switch repos/modes and trigger refresh bursts,
   then return to the editor. Note those intentional focus changes in the record.
4. Include unavailable or unauthenticated CLIs and an unavailable sandbox daemon.
   Authentication should be completed separately in a terminal; unattended Git
   credential lookup disables Git terminal and Git Credential Manager prompts.
5. Verify intentional editor, system-file and native save-dialog actions remain
   visible and functional, without an extra PowerShell console for Save.

### Windows terminal acceptance checklist

Perform each item first with Windows Terminal available, then force the
PowerShell fallback by setting `BIOME_TERMINAL` to `pwsh.exe` or
`powershell.exe` (or an executable path). That fallback uses a hidden helper
which starts the final visible interactive console; inspect the final console,
not the helper. The helper removes inherited Windows Terminal identity, but an
OS default-terminal setting can still host that PowerShell session in Windows
Terminal. An unknown `BIOME_TERMINAL` value is expected to report an error
rather than run an arbitrary recipe.

1. In regular mode, press Enter for the main worktree. Verify that it opens a
   visible terminal in the host worktree, including the spaced/quoted/Unicode
   path case. Press Enter repeatedly while launch is pending and after it is
   ready: there must be one terminal/session, not one per keypress. For a
   Windows Terminal-hosted session, the later Enter is expected to report that
   it cannot focus Windows Terminal safely; manually switch to the existing
   window and verify no duplicate was launched.
2. In the opened shell, `cd` into a subdirectory and then outside the worktree;
   change the prompt/title if convenient. Return to Biomelab and press Enter.
   For a classic visible PowerShell console with the recorded exact title and
   HWND, it should focus the original tracked session. If it is Windows
   Terminal-hosted (including OS delegation of the PowerShell fallback), expect
   the manual-switch error and no duplicate instead. Exit the shell, press
   Enter, and verify one replacement can be opened; repeat Enter to confirm
   the applicable focus/manual-switch behavior for that replacement.
3. In sandbox mode, repeat on both the main card (`sbx run`) and a linked card
   (`sbx exec` with the configured agent). Confirm each card/mode owns its own
   host terminal, and that returning to either card reuses its session after
   the sandbox command attaches or changes directory.
4. Exercise the editor and allow multiple background refresh cycles while the
   terminal is active. Verify typing remains uninterrupted and no unsolicited
   focus transition occurs. If Windows refuses a classic-console focus request,
   or the session is Windows Terminal-hosted, Biomelab should show the failure
   and must not open a duplicate terminal; switch to the existing terminal
   manually, then retry as appropriate.

Record whether Windows Terminal and the helper-created PowerShell fallback each
passed, the actual final host (classic console or Windows Terminal), which
PowerShell executable was used, any `BIOME_TERMINAL` value, and screenshots or
focus-trace timestamps for failures. Treat Windows Terminal manual switching
with no duplicate as the expected reuse result, not as full focus support. A
launcher start, a CI result, or an absence of a console flash alone is not an
acceptance pass.

Repeat the observer with `-out after-focus.csv`. Pass only if typing remains
uninterrupted and there are no unsolicited foreground transitions to Biomelab
or its descendants, whether or not consoles flash. Record the build SHA,
configuration, scenario and before/after result. A confirmed residual GUI focus
defect needs investigation even if command suppression works.

## Automated coverage and limits

The CI test matrix runs the full suite with the race detector on Linux, macOS,
and Windows. Windows executes command runtime tests through a GUI-subsystem
launcher, checks that background children have no console window, and also runs
`go test -race -count=1 -v ./internal/terminal` as an explicit native-terminal
integration check. Its `windows-terminal-<run-id>-<attempt>` artifact contains
that package's `test.log`. Cross-platform tests cover command I/O, errors,
cancellation, inherited output-pipe waits, refresh coalescing and stale results,
and terminal request/error handling. These are CI checks, not a claim that the
Windows desktop acceptance checklist has passed.

`CREATE_NO_WINDOW` applies to each configured command; arbitrary third-party
descendants can still create consoles or dialogs. The integration descendant
test applies the policy explicitly at each generation. Cancelling a command
kills its direct process and bounds waits for inherited pipes; it does not
guarantee termination of every descendant. Check configured credential helpers
and sandbox tools in the desktop trace. Automated/headless tests and the observer's
cross-compilation do not constitute a completed desktop acceptance run.

## Windows CI diagnostics

The Windows console regression writes one `process-<pid>.jsonl` file per process
when `BIOMELAB_CONSOLE_TRACE_DIR` is set. This instrumentation is confined to the
test helper and GUI-subsystem launcher. It records the launcher, policy parent,
and policy child independently: process/parent/child IDs, timestamps, launch
flags, console HWND, input/output code pages, console-process count, Windows
build, Go version, architecture, foreground HWND/PID, numeric API errors,
elapsed time, and exit stage/code. A nonzero
code page or attachment count is diagnostic; the assertion requires no console
window. Test output includes both parent and child snapshots and helper stderr
on failure.

The Windows test job disables test-result caching and uploads the trace files
plus the full suite output in `test.log` as
`windows-console-<run-id>-<attempt>`, including when the test fails. Artifacts
are retained for seven days. The separate terminal package check is uploaded as
`windows-terminal-<run-id>-<attempt>`. To inspect a run:

```sh
gh run view <run-id> --log-failed
gh run download <run-id> --name windows-console-<run-id>-<attempt> --dir ./windows-console
gh run download <run-id> --name windows-terminal-<run-id>-<attempt> --dir ./windows-terminal
```

Structured traces omit command arguments, command lines, working directories, environment
values, window titles, and credential data. These process snapshots help diagnose
CI behavior; `test.log` can additionally contain ordinary Go build/test
diagnostics and temporary paths. They do not replace an interactive foreground trace on Windows.
