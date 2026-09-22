# Sandbox workflows

BiomeLab supports host mode and recommends Docker Sandboxes for agent execution. One sandbox is associated with each agent per repository; that repository's worktrees share it. Worktrees and notes remain on the host and are visible inside the sandbox at the same paths. Agent edits to those shared files affect the host workspace; sandboxing isolates the execution environment, not the shared source files.

## Create or register a sandbox

1. Install and initialize `sbx`; run `sbx ls` in a terminal before using the GUI.
2. Select a repository in the projects panel and press `n`, or choose sandbox mode when adding a repository with `a`.
3. Choose the agent, then answer **Do you want to add kits?** **No** skips catalog discovery. **Yes** uses `gh` to discover compatible kits and opens a picker.
4. Review and confirm creation. The sandbox is created or discovered before its mode is registered; no second `n` action is needed. Canceling or failing setup leaves the previous mode unchanged.

An existing sandbox can be registered without changing its kits. Adding a sandbox to a regular-only repository replaces its regular entry. Multiple agents can have separate sandbox modes for the same repository.

## Kits

Catalog metadata comes from `docker/sbx-kits-contrib` via `gh api`. Kits are installed as Docker Hub OCI artifacts, such as `docker.io/sbx/code-server-kit:latest`. Saved metadata records the exact reference and rolling `latest` tag, not an immutable installed digest. Compatibility is filtered for the selected agent.

Kits are selectable only at creation. Adding kits later would recreate the agent container; BiomeLab does not offer that workflow or automatically recreate a sandbox.

## Worktrees and lifecycle

Creating a worktree uses the host's `.biomelab-worktrees/` directory in either mode. On the main sandbox card, `Enter` opens the primary agent session with `sbx run`. On a linked card it opens the agent in that worktree through `sbx exec`. The sandbox command is passed as an executable argument vector, not reparsed by a host shell. BiomeLab tracks the host terminal it opened for each card and mode, so a later `Enter` normally reuses that tracked session even after the sandbox command changes directory or attaches remotely. It does not discover unrelated sandbox sessions from a host working-directory scan; after restart those sessions cannot be reliably recovered. If guarded activation or inspection fails, the association is retained and BiomeLab reports the failure instead of opening a duplicate terminal.

On the main sandbox card, use `s` to start, `Shift+S` to stop, and `d` to remove the sandbox after confirmation. `n` creates a missing sandbox. Removing a sandbox stops and deletes its containers; host worktrees remain.

`x` in the projects panel removes a mode registration without deleting the sandbox. Removing the last sandbox mode restores host mode; removing the remaining host mode unregisters the repository.

See [notes and activity](notes-and-activity.md) for how shared notes and re_gent differ between host and sandbox execution.
