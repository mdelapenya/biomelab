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
rejection remains covered separately, before any POSIX launch recipe executes.
