# Terminal reuse validation (#84)

The report came from macOS ARM. The emulator, Biomelab version, `BIOME_TERMINAL`,
and regular/sandbox mode were not recorded. Automated tests run on the local
macOS AMD64 machine; cross-compilation does not establish ARM desktop behavior.

On macOS ARM, repeat these checks with Terminal.app and iTerm2 as the default
`.command` application. Record the OS, architecture, Biomelab commit, terminal
version, mode, and `BIOME_TERMINAL` value. Grant Automation permission when
requested; also check the denied-permission case.

1. In regular mode, press Enter on a card. Return to Biomelab and repeat rapidly,
   then again after more than five seconds. The same terminal tab should activate.
2. In that shell, enter a subdirectory, then a directory outside the worktree.
   Each subsequent Enter should still activate that tab. Change its title too.
3. Close the shell/tab, return to Biomelab, and press Enter. Exactly one replacement
   should open. Repeated Enter should reuse the replacement.
4. Start a terminal manually in a worktree subdirectory before pressing Enter.
   It should be discovered and activated. A shell in a nested linked worktree
   must belong to that linked card, not also the parent checkout.
5. In sandbox mode, repeat with the main card (`sbx run`) and a linked card
   (`sbx exec` / configured agent). Each card should keep its own host terminal.
   Switch modes and back; each mode should reuse its original session.
6. Deny Automation permission. Enter should show an error without another terminal
   appearing. Grant permission and retry; the original session should activate.
7. If a launch never registers, retry after 30 seconds. The recovery dialog should
   let you keep waiting or explicitly forget the session. Forgetting never opens
   a window itself. Close any previous terminal before the next Enter.

Associations last for the current Biomelab run. On restart, regular terminals
within known worktrees can be rediscovered; sandbox sessions and shells moved
outside their worktree cannot be recovered. Custom macOS emulators other than
Terminal.app/iTerm2 are launchable but do not support targeted activation here.
Linux activation requires X11/xdotool and an identifiable window; ambiguous
emulator windows report an error instead of choosing an arbitrary window.

Automated regression coverage includes real POSIX handshake/host-shell lifetime,
PID reuse, inspection/activation failure, delayed registration, path aliases,
nested worktrees, regular/sandbox reuse, mode switches, pending key repeats,
closed-session replacement, and late results after card removal. Windows launch
selection, PowerShell command encoding, tracked-session registration, and
guarded activation are covered in the Windows terminal package. That coverage
does not establish native Windows desktop behavior.

## Windows native desktop acceptance — pending

Windows now prefers `wt.exe`. Otherwise, a hidden PowerShell helper starts the
selected `pwsh.exe` or `powershell.exe` as the final visible interactive
console, rather than directly inheriting unusable null standard handles from a
GUI process. It scrubs inherited `WT_SESSION`/`WT_PROFILE_ID` values, then the
long-lived shell identifies its actual visible host; the OS may still delegate a
PowerShell launch to Windows Terminal. A sandbox command remains an argument
vector through the tracked host-terminal launch; it is not converted to a shell
string. The implementation records the host shell before `sbx run` or `sbx
exec` begins. Windows Terminal activation deliberately returns a visible
manual-switch error—there is no title/HWND guessing or `wt.exe` fallback that
could create a new window. The session remains associated, so repeated Enter
does not duplicate it. Classic PowerShell activation is available only for a
visible recorded console HWND with an exact tracked title. Any failed inspection
or focus request retains the association rather than opening another terminal.

No native Windows desktop run has been recorded yet. Use the Windows checklist
in [Windows focus regression verification](windows-focus-validation.md) before
claiming Windows launch, reuse, or foreground acceptance.

## macOS AMD64 desktop run — 2026-09-21

Tested the packaged `Biomelab.app` executable from commit `60d6d7b`
(`v0.7.0-5-g60d6d7b`) on macOS 26.6.2 (25G83), x86_64, Terminal.app 2.15.
The Intel executable was built with `task build-darwin-amd64`, packaged with
Fyne using that executable, ad-hoc signed, and verified with `codesign` and
`file`. The bundle was exercised both directly and through LaunchServices
(`open -n`). Actual keyboard/mouse events drove the GUI; Terminal window IDs,
TTYs, screenshots, typed input, and a 50 ms foreground-app trace supplied the
observations. A temporary repository with a nested linked worktree was used.

This was **not an unconditional default-startup pass**: the first two launches
failed before the `.command` script ran. Oh My Zsh's update prompt consumed the
leading slash of Terminal's queued command, producing a relative path that did
not exist. Biomelab reported an unresolved launch, suppressed duplicates, and
successfully offered its explicit forget/retry flow. To test the subsequent
session behavior, Terminal was restarted with `DISABLE_AUTO_UPDATE=true` only
in that test process's environment. No shell configuration was edited.

| Desktop check | Observed result |
| --- | --- |
| Repeat Enter on the main card | Passed: four repeats activated window 5680 / `/dev/ttys004`, without another window. |
| Change to a subdirectory, then outside the worktree; change terminal title | Passed: Enter still activated the same window and TTY. |
| Open linked card | Passed: a distinct window 5704 / `/dev/ttys006` opened. |
| Close linked shell and retry | Passed: exactly one replacement, window 5709, opened; the next Enter reused it. |
| Return to main card | Passed: original main window 5680 activated. |
| Relaunch the packaged app through LaunchServices | Passed after granting the requested macOS Automation permission: existing terminals were rediscovered, including a main shell in a subdirectory and the nested linked worktree. No extra terminal opened. |
| Leave Finder foreground across refreshes | Passed: no Biomelab activation during 37 seconds, spanning local and network refresh. |
| Type in Terminal across refreshes | Passed: all 105 test characters arrived intact over approximately 40 seconds, with no foreground-app transition during the interval. |
| Cleanup | Completed: test app and terminals closed, foreground observer stopped, original saved-config state restored, and failed launch scripts removed. |

This run does not validate Windows foreground behavior, macOS ARM, iTerm2,
explicitly denied Automation permission, or a live sandbox attachment. iTerm2
was not installed and `sbx ls` reported no sandboxes. The shell-startup prompt
failure remains a launch limitation; complete or disable interactive startup
prompts before opening terminals from Biomelab. The in-memory association and
restart limits described above also remain.
