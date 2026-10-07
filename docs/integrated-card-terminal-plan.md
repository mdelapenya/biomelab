# Integrated card terminal implementation plan

Add a persistent terminal for each card, displayed in a resizable workspace drawer. Enter and the existing Terminal actions should open or focus that card's session inside Biomelab. Card navigation must remain fast, and switching cards must preserve running shells and agent sessions.

Implementation started on 2026-10-07 after rebasing this branch onto `origin/main`
at `c222218`, which includes merged PR #94. The historical terminal reference
remains untouched. The integrated drawer now supports macOS/Linux PTY and
Windows ConPTY, with shared session lifecycle, keyboard routing, and bounded
scrollback. Windows ConPTY is app-owned through `x/sys/windows`; it requires
Windows 10 version 1809 or later, or Windows 11. The default shell resolves
`pwsh.exe -NoLogo`, `powershell.exe -NoLogo`, then `%COMSPEC%` or `cmd.exe`. If
ConPTY is unavailable, startup reports an error and the terminal menu still
offers an explicit external-terminal action. Fyne is upgraded to 2.8.1, the
terminal baseline is `v0.0.0-20260927151117-c8f30fa130e3`, and Unix PTY is
1.1.24. The terminal module uses a [documented local compatibility
patch](../third_party/fyne-terminal/BIOMELAB.md). No upstream issues, comments,
or PRs have been posted. Native Windows desktop acceptance remains pending; see
the [Windows reviewer handoff](windows-terminal-handoff.md).

The following historical research and acceptance plan predates implementation.
Its revision IDs describe the
2026-10-06 research snapshot; the merged main revision above supersedes the
pending-redesign integration step.

## Baselines and worktree

The implementation destination is the already-created `t3code/integrated-card-terminal` branch in `/Users/mdelapenya/.t3/worktrees/biomelab/t3code-9a2096dd`, rebased onto main at `c222218` (merged #94). Keep all subsequent implementation here.

The previous prototype is `origin/terminal` at `cdc81cb`. No currently registered worktree checks out that branch, so its committed changes were read with `git show` and `git diff`. This branch is historical reference only: do not merge it, port its dependency graph, or treat its architecture as the implementation baseline. Its `internal/gui/termpanel.go` illustrates session reuse and an embedded viewport; its `docs/upstream-fyne-terminal-plan.md` supplies investigation leads that require fresh verification. The prototype predates current sandbox launch and external-terminal reuse behavior.

The design driver is `feat/biomelab-ui-polish` at `d6c1357`, in sibling worktree `t3code-f66a7b47`. Its plan is in the shared Git directory at `worktrees/t3code-f66a7b47/the-mister/biomelab-ui-polish/plan.md`. The final plan entries supersede early visual experiments: OrbStack/Orca-inspired whole-app styling, Board/List/Grid, a prominent pinned Main checkout, inspector, keyboard hints, and a resizable repository sidebar. Relevant source includes `design.go`, `shell.go`, `workspace_layout.go`, `workspace_inspector.go`, `dashboard.go`, and the keyboard routing tests.

The accepted redesign is now included through the rebase onto `c222218`. Its current main implementation is the integration baseline; the sibling worktree remains read-only reference. Do not copy old dashboard geometry over the redesign.

## Dependency upgrade and adaptation to the current project

Start implementation by refreshing the project baseline and dependency inventory. The revisions recorded here are research snapshots, not permanently selected versions. Recheck upstream releases and fixes when implementation starts; prefer supported stable versions where they provide the required APIs. If the terminal library requires a pseudo-version or a patch, pin its exact revision and record why it is needed.

Evaluate and update Fyne core, the terminal widget, and the PTY/ConPTY dependencies as a compatible set. Inspect the resulting module graph and indirect GUI dependencies; do not copy the old branch's `go.mod` or run an indiscriminate project-wide upgrade. Retain the current project's other dependencies and toolchain unless compatibility requires a documented change. Use the current checkout's `go.mod`/`go.sum`, run module tidy after adding real imports, and review the resulting diff.

Validate the dependency bump against the accepted redesign before adding the terminal UI. Build the existing application and run its keyboard routing, modal lifecycle, sidebar resizing, theme/zoom, and workspace layout regressions. Record this result separately from terminal-specific tests so framework regressions can be distinguished from integration defects. Recheck the upstream findings below against the chosen versions; retire obsolete workarounds rather than carrying them forward.

Adapt the feature to these current contracts:

| Current project contract | Integration requirement |
| --- | --- |
| `internal/gui/terminal_action.go` owns deliberate external open/reuse behavior | Preserve it for the external fallback; share target resolution without mixing external session handles with embedded PTY ownership |
| `terminalTarget` includes worktree, mode, sandbox, and agent context | Carry those semantics into a stable embedded-session key; do not revert to the prototype's branch-name keys |
| `sandbox.RunAttachArgs`, `ExecAgentArgs`, and `ContainerPath` select current launch behavior | Reuse current argument builders and their tests; do not import old sandbox continuation flags |
| The redesign exposes Main and inspector terminal callbacks alongside Enter | Route all entry points through one integrated controller and preserve existing keyboard navigation |
| Dashboard/shell content is rebuilt for refresh, theme, zoom, and view changes | Keep session ownership outside rebuildable content and reconnect the view without restarting processes |
| Current platform support includes Windows external terminals | Keep explicit external open/reuse available alongside the app-owned ConPTY transport |

The first implementation deliverable is an updated dependency matrix with exact versions, required patches, supported platforms, and passing baseline checks. The old prototype is consulted only after these current contracts are established.

## Interaction and visual design

- Enter on a card, the inspector's Open Terminal action, and Main's Terminal action share one controller. Opening is deliberate; selecting a card never starts a process.
- Once the drawer is open, selecting another card displays its existing session or an empty state with an Open Terminal action. The previous session keeps running. Selection outside the drawer does not steal keyboard focus back into it.
- The header identifies repository, branch/card, mode, and agent where relevant. Show Starting, Running, Exited, or Failed state, with exit status and a concise error. Keep exited output available until Restart or explicit disposal.
- Provide Hide, Expand/Restore, and a compact session menu containing Restart, Stop session, and Open in external terminal. Hiding never terminates a session. Restart stops and reaps the old process before starting another. Stopping a busy session should make the consequence explicit through the normal dialog mechanism.
- Keep the external-terminal action as an explicit fallback using today's reuse controller. Label it as a separate terminal; it cannot migrate a running PTY or capture an existing OS terminal session.
- Place the drawer below the workspace browser/inspector area, inside the right-hand workspace. Preserve the sidebar, workspace toolbar, and prominent Main card in normal split mode. Expanded mode temporarily replaces the browser/inspector region; keep a visible Restore action and card identity. Do not embed a miniature terminal in every card or open a modal/new OS window.
- Start with roughly 40% of the available body height. Clamp to useful browser and terminal minima; when both do not fit, use expanded mode. Retain the user's preferred height independently of the current clamped height, as the redesigned sidebar does.
- Reuse the existing palette, `scaledSize`, spacing, radius, and typography helpers. Terminal cells use a monospaced font and a readable ANSI palette in both themes. Use restrained borders and labeled controls; the redesign explicitly rejected unsolicited hover popups.
- Preserve Board/List/Grid, inspector visibility, sidebar resizing, wrapping keyboard hints, and the Main card's actions. Theme, zoom, refresh, view changes, and repository switches must not recreate live sessions.
- First scope: one live session per card/context during the app lifetime. Multiple terminal tabs, durable terminal history, and automatic agent conversation resume after app restart are separate features. An embedded shell is not itself a provider conversation identifier.

Proposed geometry:

```text
Repository sidebar | Workspace toolbar
                   | Prominent Main checkout
                   | Board / List / Grid       | Inspector
                   |---------------------------------------- drag
                   | Terminal · repository / card · mode
                   | [terminal viewport]       Hide / Expand / menu
                   | Contextual keyboard hints
```

## Architecture and session ownership

Use a controller owned by `App`, created once outside rebuildable dashboard content. Separate process transport from terminal rendering. Suggested files are `internal/embeddedterminal/session.go`, `manager.go`, `pty_unix.go`, `pty_windows.go`, and `internal/gui/terminal_drawer.go`, `terminal_controller.go`, `terminal_input.go`. Keep `internal/terminal` as the existing external-terminal implementation.

Use a structured session key containing canonical repository identity, canonical worktree path, mode, sandbox identity, and agent identity. Do not key by branch, card index, or a `repoEntry` pointer. Apply platform-aware path normalization without conflating distinct worktrees. If provider-session selection is introduced, include that explicit identity too.

Model `Starting -> Running -> Exited/Failed`, plus `Stopping`. Reserve the key before asynchronous startup to coalesce repeated actions. Each start gets a generation token; every completion and queued UI callback verifies both key and generation. An old exit must not delete a replacement session or clear a newly selected terminal.

The transport owns the child process, PTY/ConPTY, resize delivery, cancellation, and exactly one wait/reap operation. Prefer `RunWithConnection` for both local and sandbox commands so lifecycle ownership is consistent. Never hold controller locks while writing to a PTY, stopping a process, or dispatching UI work. Process creation and termination run off the UI thread; UI mutations use `fyne.Do`. Shutdown must not block the UI while a renderer worker waits in `fyne.DoAndWait`.

On Unix, close the PTY and terminate the owned process group with bounded graceful shutdown and escalation; verify foreground children also stop. On Windows, implement ConPTY and appropriate owned-process cleanup behind build tags. A Unix-only `creack/pty` call in a shared GUI file is not a portable implementation. If Windows integration cannot pass its checks initially, expose a clear external fallback there and document that limitation instead of claiming parity.

Register resize handling before rendering can emit the initial size. Start with a valid nonzero rows/columns value. Coalesce subsequent resizes to the newest size, clamp conversions, deduplicate unchanged dimensions, and send the final size after a drag. Upstream Config delivery can drop notifications, so a buffered listener alone is insufficient to guarantee the final PTY size. Include explicit final reconciliation or a tested adapter hook.

Use bounded scrollback per session and coalesced refresh work. Hidden sessions must continue draining output without monopolizing the UI. Detach listeners on disposal according to the selected dependency's ownership contract; current upstream `RemoveListener` closes its channel, so the caller must not close it again.

Window close currently hides to tray: retain sessions. Actual Quit cancels starts, stops owned children, closes transports, and drains cleanup within a bounded interval. Worktree deletion, repository removal, and sandbox removal need controller hooks. Capture the original target before asynchronous operations; do not read a newly selected mode in their completion callbacks. Avoid treating one transient refresh omission as proof that a worktree was deleted.

## Command semantics

Regular mode starts the configured/default interactive shell with `cmd.Dir` set to the worktree. Pass executable/arguments directly rather than composing an interpolated shell command. Preserve an appropriate environment with terminal capabilities that the chosen emulator actually supports.

Reuse current `sandbox.RunAttachArgs` for Main and `sandbox.ExecAgentArgs` for a linked worktree, including `ContainerPath` and agent identity. Use `wt.IsMain`, not index zero, to choose behavior. Do not restore the prototype's `sbx run --branch ... -- -c`: it predates the current path-based launch logic and assumes a continuation flag shared by all agents. Do not silently relaunch an agent because a card is selected or a session exits.

If a card already has an external terminal, the integrated action starts a distinct session. Keep that distinction visible; do not promise to attach to an arbitrary existing shell or infer an agent conversation from its working directory.

## Keyboard and focus contract

Actual canvas focus is authoritative. Synchronize application focus state when clicking the terminal, opening a dialog, hiding the drawer, changing cards, and restoring focus. Route keys through the focused terminal widget/adapter; canvas-level shortcuts alone are insufficient.

Plain Tab, Shift+Tab, Escape, arrows, Enter, and terminal control chords belong to the terminal while it is focused. Preserve shell completion, vim Escape, Ctrl-C, Ctrl-D, Ctrl-Z, Ctrl-L, and tmux prefixes. Reserve a documented explicit chord to return to the board, proposed Ctrl+Shift+Space, implemented in the focused adapter and tested on each platform. Clicking the board is another escape route. Outside terminal focus, retain the redesign's navigation and view shortcuts.

Keep platform clipboard conventions: Cmd+C/V on macOS, Ctrl+Shift+C/V on Linux/Windows, and Ctrl+C as interrupt. Verify selection, Unicode, IME input, bracketed paste, application cursor mode, and modified navigation through actual Fyne event dispatch. Do not emit both a shortcut sequence and a TypedKey sequence for one physical key.

Fyne 2.7.3's `internal/driver/glfw/window.go` sends KeyDown to a focused `desktop.Keyable`; the canvas callback is the unfocused fallback. It also gives a focused `fyne.Shortcutable` precedence over canvas shortcuts. The prototype's comments saying the canvas always intercepts keys are incorrect for this version. Its focus enum can diverge after a mouse click, and its canvas word-motion workaround may never run. This is primarily an application integration problem, not evidence that Fyne needs a global interception patch.

## Findings in the old prototype

| Finding | Consequence | Required change |
| --- | --- | --- |
| `scrollForwarder.Scrolled` sends Up/Down to the PTY unconditionally | Scrolling can edit shell history or move a TUI cursor instead of reading output | Implement actual scrollback; honor mouse reporting and alternate-screen semantics |
| `onSessionEnd(key)` deletes by key, then queues an unconditional placeholder update | A late exit can remove a replacement or overwrite a different visible session | Verify generation and active view again inside the UI callback |
| Shell cleanup calls `Exit`, which writes Ctrl-D | Busy programs need not exit; shutdown is not guaranteed | Own cancellation and transport/process teardown |
| Sandbox cleanup kills the direct child without calling `cmd.Wait` | Child resources are not reliably reaped; descendants may survive | Exactly one wait plus process-tree cleanup |
| `buildContent` creates a fresh panel | Content reconstruction can discard ownership of existing sessions | App-scoped manager independent of render tree |
| Sandbox startup invokes `pty.Start` synchronously; run errors are ignored or only logged | UI stalls and unexplained blank terminals | Asynchronous startup and visible state/errors |
| Keys omit some repository/mode/agent identity | Context changes may reuse the wrong session | Structured context key |
| Canvas shortcut fixes depend on an inaccurate dispatch model | Focus and word navigation are unreliable | Focused adapter and native-path regression tests |
| Shared GUI file imports Unix PTY operations | Windows behavior is not implemented by this path | Separate platform transports |

These are source-review findings, not claims of a live reproduction. In particular, the prototype intends Tab/Escape to leave the terminal, but Fyne's focused routing may bypass that code entirely; either behavior fails its intended focus contract.

## Upstream Fyne terminal findings

Checked upstream through GitHub on 2026-10-06. Source observations below use `fyne-io/terminal` commit `c8f30fa130e35b342233cf2ee3171d3a161039bd`, rather than assuming the old prototype's pinned `73c54fbd1dd3` is current. These concerns mostly belong to the terminal library, not Fyne core.

| Area | Evidence and current status | Implementation decision |
| --- | --- | --- |
| Modified keys | [`input.go`](https://github.com/fyne-io/terminal/blob/c8f30fa130e35b342233cf2ee3171d3a161039bd/input.go) still emits Shift+Up as `ESC [ A ; 2`; cursor encoding ignores Alt/Ctrl state. Modified arrows also encounter the separate `TypedShortcut` path. | Add byte-level tests through both real dispatch paths; fix the encoder in an isolated dependency patch if needed. Do not merely copy a canvas binding. |
| Scrollback | [PR 147](https://github.com/fyne-io/terminal/pull/147) is open and unmerged. The inspected terminal type has no `Scrolled` implementation. | Usable bounded scrollback is a release requirement. Evaluate the PR and maintain a narrowly scoped pinned patch if necessary; never substitute unconditional arrow input. |
| External PTY resize | [Issue 103](https://github.com/fyne-io/terminal/issues/103) remains open. [`term_unix.go`](https://github.com/fyne-io/terminal/blob/c8f30fa130e35b342233cf2ee3171d3a161039bd/term_unix.go) returns early from built-in resize when `t.pty` is nil. | Forward dimensions for caller-owned transports. Config listeners are available, but need reliable final-size handling. A direct resize hook is an upstream API improvement, not a prerequisite to experimenting. |
| Backspace | [Issue 138](https://github.com/fyne-io/terminal/issues/138) remains open; the encoder sends `0x08`, while the reported terminal settings expect DEL. | Test actual shell/PTY erase configuration. Choose a consistent configurable/default mapping; do not claim every shell fails. |
| Resize cost | [PR 136](https://github.com/fyne-io/terminal/pull/136) is closed without merging. [`Resize` and `guessCellSize`](https://github.com/fyne-io/terminal/blob/c8f30fa130e35b342233cf2ee3171d3a161039bd/term.go) still measure a text cell on each resize invocation. | Profile divider drags with output-heavy TUIs. Measurement is confirmed; a user-visible freeze and its dominant cause are not. Cache only with theme/font/scale invalidation and evidence. |
| Close lifecycle | Current [`Terminal.Close`](https://github.com/fyne-io/terminal/blob/c8f30fa130e35b342233cf2ee3171d3a161039bd/term.go) stops/hangs up the connection; `Exit` still sends Ctrl-D. | The old claim that Ctrl-D is the only available shutdown path is obsolete. Use Close where appropriate and retain ownership of caller-started process waits/descendants. |
| Dependency baseline | Current [`go.mod`](https://github.com/fyne-io/terminal/blob/c8f30fa130e35b342233cf2ee3171d3a161039bd/go.mod) requires Fyne `v2.7.5-0.20260529084154-f5f48d2ab76e`; Biomelab uses `v2.7.3`. | Pin an evaluated compatible pair. Treat the Fyne upgrade as an explicit integration change and rerun the redesign's focus, layout, dialog, and theme checks. |

The older upstream plan overstates two conclusions: wheel-to-arrow forwarding is not general-purpose terminal scrolling, and a closed performance PR does not establish maintainer rejection or prove a specific bottleneck. Do not reuse its proposed upstream messages without updated evidence.

For any future upstream report, first capture a minimal reproducer, dependency SHA, OS, expected/actual input bytes or dimensions, and a failing test/profile. Check for duplicates immediately before filing. For keyboard behavior, cover normal and application cursor modes plus the real shortcut path. For resize, demonstrate final-size loss or provide a profile. Posting issues/PRs is outside this read-and-plan task.

## Implementation sequence and acceptance

1. **Current baseline, dependency upgrade, and feasibility spike.** Establish the current project contracts and compatible dependency set described above, then validate the existing app/redesign on the upgraded framework. In an isolated harness, evaluate the selected terminal/Fyne pair, focused keyboard adapter, real scrollback, Unicode/ANSI rendering, clipboard/paste, custom PTY resize, and Close. Test an interactive agent TUI, bash/zsh, vim/less, and tmux. Produce reproducible tests for confirmed upstream defects. Decide the exact pinned dependency/patch set before full integration; stop short of shipping a drawer with unusable scrolling or input.
2. **Session manager and platform transport.** Add structured keys, generation-checked state transitions, asynchronous launch, cancellation, bounded output/history, latest-size delivery, and process cleanup. Prove repeated opens start once, distinct modes stay isolated, old completions cannot delete replacements, and shutdown during startup leaves no child behind. Cover a child that ignores EOF and a child with descendants.
3. **Command selection.** Share current regular/sandbox targeting rules with external launch. Test Main versus linked worktrees, different agents, stopped sandboxes, paths with spaces/quotes, Windows path translation, missing executables, and startup failures. Preserve explicit external reuse behavior and its tests.
4. **Redesign integration.** Bring the accepted UI baseline into this worktree. Mount a persistent drawer through `app.go`/`shell.go` and the workspace body layout. Connect Main, inspector, and Enter actions. Add state/error controls and retain sessions across repository, theme, zoom, refresh, and Board/List/Grid switches. Keep the sidebar width and pinned Main layout regressions passing.
5. **Focus and shortcuts.** Implement the focused adapter and return-to-board action. Test pointer focus, Tab completion, Escape in vim, control chords, modifier combinations, paste, modal entry/exit, drawer hide, and card switch. Tests must exercise Fyne's actual routing order, supplementing encoder unit tests with live native checks.
6. **Lifecycle and removal flows.** Wire tray hide versus Quit, worktree deletion, repository removal, sandbox removal, failed deletion, restart, and exit while a different card is visible. Ensure workers cannot publish to disposed widgets or block shutdown waiting for UI work. Preserve final output and actionable errors.
7. **Validation and documentation.** Run focused tests, relevant GUI/backend race tests, the repository build/lint tasks, and existing terminal/sandbox regressions. Exercise native macOS and Linux; validate ConPTY on Windows or clearly document the temporary external fallback. Update dashboard/architecture/known-limitations documentation and screenshots for the implemented scope.

Completion requires one process per deliberately opened card/context, no OS window for supported integrated actions, uninterrupted sessions while switching views, functioning terminal input and scrollback, correct final resize, and no owned-process leaks after Stop/Quit. Inspect actual light/dark, narrow-window, zoomed, Board/List/Grid, and expanded-terminal renders. Profile rapid resize and sustained output; confirm hidden sessions do not freeze navigation and memory stays bounded by the configured session/history limits.

The initial documentation review did not execute application tests. Implementation validation includes the full Go suite, GUI/backend race tests, terminal compatibility tests, and rendered drawer inspection. Native Windows desktop acceptance and interactive agent/TUI acceptance on Linux and macOS ARM remain outstanding; see the [known limitations](known-limitations.md) and [Windows reviewer handoff](windows-terminal-handoff.md).
