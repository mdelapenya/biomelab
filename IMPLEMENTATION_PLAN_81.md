# Issue #81: Stop Windows from constantly losing focus to Biomelab

Status: implementation and regression coverage added in this worktree; final
interactive Windows reproduction/acceptance remains pending.

## Implementation record

- Added a shared background command policy using `CREATE_NO_WINDOW` on Windows
  and migrated noninteractive CLI helpers, including the PowerShell save helper.
  Each descendant must opt into that policy; it is not inherited as a creation flag.
- Added bounded refresh lanes, immutable run contexts and generations, cancellation
  through refresh operations, cached CLI preflight across resume, and permanent
  stop. Explicit network refresh rechecks CLI availability. Late callbacks check
  their run again on the UI thread. Quick snapshots preserve existing detection.
- Bounded Git credential/probe operations; disabled unattended Git and Git
  Credential Manager prompts. Command pipe waits are bounded after exit/cancel;
  arbitrary third-party process trees are not forcibly terminated.
- Added terminal request coalescing, visible errors, Windows rejection before
  incompatible custom launch, bounded activation helpers, launcher reaping and
  shell-safe terminal titles. Activation rechecks live CWD/process ancestry;
  results for removed worktrees/repos are discarded. Immediate launcher failures
  within a short observation interval are reported. Native Windows terminal
  support remains separate.
- Added command, credential, refresh and terminal regressions, plus native Windows
  command/terminal CI execution and a GUI-subsystem console test launcher.
- Added a Windows foreground event observer and
  [desktop verification procedure](docs/windows-focus-validation.md).

The original staged plan and baseline findings follow. The Sol exploration found
no source evidence that normal dashboard refresh calls Show/RequestFocus. The
implemented subprocess/lifecycle fixes address confirmed defects; their causal
role in the reported constant focus loss still needs the before/after Windows
foreground trace. Do not close #81 solely on automated test results.

Source: [issue #81](https://github.com/mdelapenya/biomelab/issues/81),
“Windows: the focus is constantly lost to the GUI”. The primary complaint is
constant loss of keyboard focus, making it impossible to keep working in another
application. The issue also reports terminals opening and immediately closing.
The user observed repeated millisecond-lived
windows on a Windows machine on September 17, 2026, and asked that the roughly
six-month-old window management code be reassessed rather than assumed correct.

Prepared September 18, 2026 against `0a9bd28849eec1c0b22dce5d492f8564df2e3a9e`,
in the existing worktree `/Users/mdelapenya/.t3/worktrees/biomelab/t3code-bee7f943`,
branch `t3code/fix-window-manager-loop`.

## Recommended scope

First trace where keyboard focus goes and what causes each unsolicited change.
Correlate those transitions with the flashing windows, background processes and
GUI lifecycle events. Console suppression is a candidate fix, not proof that
focus loss is resolved. If focus moves to Biomelab independently of helper
consoles, fix the responsible GUI/activation path as part of this issue.
Harden the refresh lifecycle and terminal action error handling where they
contribute to the failure.
Keep native Windows terminal support a separate follow-up unless reproduction
shows it is necessary to resolve the reported failure.

The current code does not contain a standalone window manager. The relevant
behavior spans process execution, refresh scheduling, terminal detection and
activation, Fyne event handlers, and tray/window lifecycle. Replacing the GUI
toolkit or upgrading dependencies solely because of their age is not justified
by the evidence so far.

## Findings and hypotheses

These describe the baseline source before the changes above, not a reproduction
on the affected Windows machine.
The terminal open/reuse implementation last changed in commit `49c1dd9` on
April 24, 2026; the GUI refresh implementation also dates to April 2026.

| Area | Observed implementation | Implication |
| --- | --- | --- |
| Background processes | Production `exec.Command` / `CommandContext` calls do not configure `SysProcAttr` for Windows. | Leading hypothesis: short-lived CLI consoles flash when launched by the packaged GUI. Redirected output alone does not establish a no-console launch policy. |
| Refresh cadence | `internal/gui/refresh.go` starts a network refresh immediately, local refresh every 5 seconds, and network refresh every 30 seconds by default. | Capture both startup bursts and periodic events; millisecond window lifetime does not establish a millisecond timer. |
| Command fan-out | Provider PR lookups allow four concurrent operations, one lookup per branch; sandbox local refresh calls `sbx ls` and `sbx version`; fetching can call `git credential fill`. Startup also probes tools and bootstraps re_gent. | Multiple console flashes per cycle can look like continuous reopening. Regular and sandbox modes need separate traces. |
| Refresh lifecycle | `Pause` cancels ticker contexts, but refresh operations create background contexts or use non-context commands. Manual triggers spawn independent goroutines. | In-flight work can outlive a pause or overlap a resumed cycle. This can amplify subprocess activity; it is not yet proven to cause the report. |
| Windows terminal actions | `terminal.Open` / `OpenWithTitle` have no Windows backend. A custom `BIOME_TERMINAL` is invoked with POSIX `-e sh -c` arguments. Windows activation returns false. | A custom configuration can fail immediately; a default launch returns an unsupported-platform error. This is a distinct hypothesis for action-triggered flashing. |
| Action results | `handleEnter` discards launch errors and the final activation result; no in-flight action guard exists. | Failures are silent, and repeated Enter events can request multiple launches before detection catches up. There is no explicit automatic relaunch loop in this handler. |
| Detection | `internal/terminal/detect.go` compares cleaned CWDs exactly, uses substring shell matching, and infers ownership from PPID ancestry. | Windows path casing, inaccessible CWDs, terminal hosting, and stale PIDs need review before claiming reliable reuse. Detection currently does not launch or activate windows. |
| GUI focus | Refresh application rebuilds widgets; explicit `RequestFocus` calls found are in user-invoked note/log windows. Tray Show is user-invoked. | Do not equate canvas selection changes with OS foreground changes. Trace actual foreground ownership before blaming Fyne. |
| Existing verification | CI runs tests on Linux and packages on Windows, where verification only invokes `--version`. | Windows packaging success does not validate interactive focus or console creation. |

The pinned gopsutil Windows process implementation uses Windows APIs for the
inspected enumeration/enrichment paths; no shell-based enumeration call was
found in `process_windows.go`. Inspect dependency descendants if the trace points
there, rather than replacing process detection speculatively.

## 1. Establish a Windows reproduction

1. Record the failing executable version/commit, Windows version, launch method,
   regular/sandbox mode, repo/worktree counts, default console host, and relevant
   `BIOME_TERMINAL` / `BIOME_REFRESH` settings. Compare the reported build with
   the current packaged build; the reporter's exact binary is not yet known.
2. Launch the packaged `.exe` from Explorer, then type in another application.
   Record each interruption and its destination: Biomelab's main window, a
   Biomelab dialog, a child console, or another window. Also test typing within
   Biomelab to distinguish OS foreground loss from widget focus changes.
   Repeat while Biomelab is minimized and hidden to the tray. Compare launch
   from an existing console, which can change console inheritance behavior.
3. Capture process creation/exit with Microsoft's
   [Process Monitor](https://learn.microsoft.com/en-us/sysinternals/downloads/procmon),
   including the Biomelab descendant tree, executable names, PIDs, parent PIDs,
   timestamps and exit status. Correlate that with a foreground-window event
   trace recording HWND/PID, plus a screen recording if useful. Process creation
   alone does not prove which window took keyboard focus.
4. Separate idle startup, idle periodic refresh, manual refresh, repo/mode
   switching, dependency re-check, and a single Enter action. Use a fresh config
   and a representative multi-worktree config. Exercise installed, absent and
   unauthenticated CLI states, and unavailable sandbox services.
5. If needed, add opt-in file diagnostics: operation label, refresh generation,
   executable basename, PID, duration, exit code and launch/activation result.
   Do not log credential input/output, tokens, or full command payloads. A GUI
   diagnosis must not open a logging console itself.

Exit criterion: a repeatable focus-loss trigger, destination HWND/PID and
correlated process/GUI events are recorded. If focus loss correlates with child
consoles, proceed with phase 2 and repeat the foreground trace after suppression.
If it occurs only on terminal actions, prioritize phase 4. If Biomelab changes
foreground without child consoles, isolate the Fyne/tray path in a minimal
reproduction before choosing a toolkit fix. Disappearance of flashing windows
alone does not satisfy this criterion. Evidence determines patch order.

## 2. Give background subprocesses an explicit Windows policy

Introduce a small `internal/command` package, separate from the existing process
enumeration package. Expose a context-aware background command constructor with
the normal `*exec.Cmd` configuration surface for working directory, environment,
input/output and pipes. Keep time budgets with callers rather than imposing a
single deadline on every operation.

- Implement platform behavior in `command_windows.go` and a `!windows` file.
  Windows background console commands use `CREATE_NO_WINDOW`; configure
  `HideWindow` where appropriate. Preserve compatible existing process attributes
  and reject conflicting console flags rather than silently combining them.
  Microsoft documents that `CREATE_NO_WINDOW` is ignored with `CREATE_NEW_CONSOLE`
  or `DETACHED_PROCESS`, and for non-console applications.
  See [process creation flags](https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags).
- Migrate by execution intent, not merely by package or user/background trigger.
  A user-requested PR lookup still needs a hidden CLI; a requested terminal must
  remain visible. Use structured arguments, preserving stdin, stdout/stderr,
  environment and existing error semantics.
- First migrate refresh/startup paths: `internal/provider/{github,gitlab}.go`,
  `internal/git/credential.go`, `internal/sandbox/sandbox.go`,
  `internal/sysdeps/checks.go`, and `internal/regent/init.go`.
  Audit the remaining production calls in `internal/github`, `internal/kits`,
  `internal/regent`, and `internal/git/worktree.go` so manual actions cannot
  reintroduce helper console flashes.
- Classify editor/system openers, terminal launchers and the PowerShell save
  dialog separately. For a native save dialog, suppress the helper console while
  preserving the requested dialog. Do not blanket-hide interactive applications.
- Make unattended credential lookup noninteractive and bounded, checking the
  configured helper's behavior. `GIT_TERMINAL_PROMPT=0` alone is not proof that a
  third-party credential helper cannot show its own UI. Return an authentication
  state for the existing UI instead of starting an unattended login interaction.
- Verify descendants as well as direct children. The parent's flags do not
  guarantee that a CLI cannot explicitly create another console or GUI. Any
  additional descendant fix must follow the reproduction evidence.

Do not treat the executable's GUI subsystem flag, a longer refresh interval, or
a PowerShell `-WindowStyle Hidden` wrapper as the fix for child console creation.

## 3. Make refresh lifecycle bounded and observable

Update `internal/gui/refresh.go` and the operations it calls:

- Capture an immutable context and run-generation token when starting a manager.
  Propagate cancellation into refresh operations, process enrichment, provider
  requests and background commands; use the Go Git cancellation APIs available
  in the pinned dependency for fetches.
- Serialize/coalesce refresh requests per manager, allowing one pending request
  per kind rather than a goroutine per trigger. Preserve the quick first render
  and ensure a slow network operation does not unnecessarily block local updates.
  Local and network work may use separate bounded slots.
- On pause/stop, cancel active work and reject late results from that run. Check
  the generation again inside queued `fyne.Do` callbacks. Preserve the existing
  repository mutation generation check: it solves a different stale-data problem.
- Set operation-specific deadlines for probes and network lookups. Check shutdown
  and timeout behavior for CLI descendants/held pipes; do not assume cancelling
  an `exec.Cmd` always terminates its entire process tree.
- Do not add retries that open windows, reset focus, or run faster on failure.
  Hidden/minimized refreshes must remain nonintrusive whether polling continues
  or a future power-saving policy pauses it.

This can ship independently after the console fix; failure to reproduce overlap
must not block a small, verified Windows console patch.

## 4. Reassess terminal actions and window focus

In `internal/gui/shortcuts.go`, `internal/ops/worktree.go`, and `internal/terminal`:

- Return an explicit outcome for opened, activated, unsupported, target missing,
  and failed actions. Surface failures through the originating repo's status UI
  on the Fyne thread without raising the main window or opening repeated dialogs.
- Add per-target in-flight state keyed by repo, worktree and mode. Coalesce Enter
  while an action is pending; handle key repeat and the delay before a launched
  terminal appears in detection. Clear state on failure, bounded expiry or
  confirmed detection. A retry after failure requires a new deliberate action.
- Keep refresh/detection free of launch and activation side effects. Recheck stale
  process identity before activation; activation failure must not trigger a loop
  that opens replacement terminals or forces Biomelab back to the foreground.
- Validate Windows custom-terminal capabilities before spawning. Known Windows
  terminal executables must not receive the generic POSIX argument recipe.
  Until a supported adapter exists, report the limitation clearly; do not imply
  that setting `BIOME_TERMINAL=wt` makes the current implementation work.
- Preserve supported macOS/Linux launch and reuse behavior. Test title/path
  quoting while touching command construction, including spaces, apostrophes,
  Unicode and shell metacharacters. Ensure started launcher processes are reaped
  without making the GUI wait for an interactive terminal session to exit.

Native Windows launch/reuse is a separate capability project unless the trace
requires it for #81. It should use OS-specific adapters, structured command
arguments (including the sandbox call path, which currently builds POSIX shell
strings), a documented shell fallback, and a shell that remains open on errors.
Prototype Windows Terminal tab/window identity before promising PID-based reuse;
do not assume its launcher PID is the interactive shell/window PID. Cover CWD
access failures, path casing and stale process identity in that design.

Any native foreground activation must be user-triggered and accept refusal:
Windows restricts foreground changes and can reject them even when eligibility
conditions appear satisfied. Never use repeated focus forcing as recovery.
See [SetForegroundWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setforegroundwindow).

## 5. Regression tests and release acceptance

Add tests where they prove behavior rather than just assert flag assignment:

| Layer | Required verification |
| --- | --- |
| Windows subprocess integration | A GUI-subsystem test launcher runs a console helper that reports console attachment, arguments, CWD, environment, stdin and output. Verify no console attachment for background execution, preserved I/O/errors, and bounded timeout behavior. Include a descendant case. |
| Refresh lifecycle | With controllable slow operations, burst manual triggers, pause/resume and stop. Assert bounded concurrency, cancellation, no stale callback application and no work scheduled after stop. Run race tests. |
| Terminal action controller | One pending launch per target; repeated Enter does not multiply launches; launch/activation failure is visible once; unsupported Windows configuration fails before spawn; stale detection and repo switching cannot misdirect results. |
| Visibility boundary | Requested terminal/editor/save-dialog actions remain usable while their helper consoles stay suppressed. No refresh result calls launch, Show or RequestFocus. |
| Compatibility | Existing macOS/Linux terminal/detection tests pass; new platform-neutral cases cover quoting and error propagation. Windows-specific detector cases run on Windows. |

Extend `.github/workflows/ci.yml` with Windows execution of the new subprocess and
noninteractive regression tests, retaining Linux race coverage and packaging on
all supported platforms. Cross-compilation and `--version` remain useful build
checks, but neither substitutes for interactive acceptance.

On an interactive Windows desktop, test the actual packaged executable for at
least five minutes per representative regular/sandbox configuration, spanning
multiple local and network ticks. Type continuously in another application,
minimize and hide Biomelab, switch repos/modes repeatedly, trigger refreshes,
and exercise dependency failures and supported explicit window actions.

Release criteria:

1. Continuous typing in another application remains uninterrupted: zero
   unsolicited foreground changes attributable to Biomelab or its descendants,
   including when no console flashes are visible. Confirm with before/after
   foreground traces. Also require zero unsolicited console windows.
2. Refresh data still updates; CLI/authentication failures remain diagnosable.
3. Explicit terminal actions either perform one supported action or show one
   actionable failure. They do not retry or relaunch automatically.
4. Pause/resume/quit leave no unbounded refresh work; delayed results cannot
   update an obsolete repo/mode run.
5. Windows runtime tests, full `go test -race ./...`, lint and packaging checks
   pass. Record the Windows version, build SHA and before/after trace outcome.

## Delivery and documentation

Deliver a focus-loss reproduction first, then small reviewable patches in the
order justified by the trace. The initial candidate sequence is background
command policy, refresh lifecycle hardening, then terminal outcome/reentry
handling; a confirmed GUI focus defect takes priority over unrelated hardening.
Each patch includes its regression coverage. A verified console fix can ship
before broader work, but #81 remains open if focus loss persists. Do not claim
full Windows terminal support as part of that release.

Update [known limitations](docs/known-limitations.md),
[configuration](docs/configuration.md), and [architecture](ARCHITECTURE.md) to
describe the actual Windows behavior and launch-policy boundary. Record evidence
and validation in the eventual PR; close #81 only after the reported Windows
symptom is verified resolved. Toolkit/dependency changes or native terminal
support need their own evidence and acceptance criteria.

## Planning validation and remaining evidence

The issue, current code, relevant Git history, pinned Go/gopsutil implementation,
CI workflows and Microsoft process/focus documentation were inspected. The
affected Windows machine has not been reproduced from this macOS worktree.
The user could not confirm whether flashing occurred while idle or only after
a terminal action, so both cases remain required. They subsequently emphasized
that constant focus loss was the complaint; flashing is an associated observation
whose causal role remains unconfirmed. Exact failing build, terminal
configuration and foreground/process trace remain to be collected during
implementation.

Planning checks: `go test -race ./...` passed on macOS, with linker warnings
about duplicate `-lobjc` libraries and malformed `LC_DYSYMTAB`. All local Markdown
link targets in this plan exist. These checks establish a baseline; they do not
validate the proposed fix or Windows foreground behavior.

Implementation validation: the full `go test -race ./...` suite passed, followed
by targeted terminal/GUI race tests after the final launcher refinements. The
macOS executable builds successfully. golangci-lint v2.11.3 reports zero issues;
it was run through `go run` because the installed lint binary targets arm64 and
this session uses amd64. Windows command/terminal/Git/ops/provider test binaries,
the GUI-subsystem test launcher and the foreground observer cross-compiled.
Changed documentation's local link targets and diff whitespace checks pass.
Existing macOS linker warnings remain unchanged.

Native Windows CI execution and a packaged Windows desktop acceptance run were
not performed from this macOS session. The [verification guide](docs/windows-focus-validation.md)
is the remaining handoff for confirming the reported focus loss is resolved.
Terminal activation remains best-effort between validation and the OS call;
the launch cooldown expires after five seconds, and launcher failures occurring
after the short observation interval cannot be synchronously reported. These
limits do not introduce an automatic launch or focus-recovery loop.
