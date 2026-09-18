# Notes, PR drafts, and agent activity

## Task notes and PR drafts

Select a worktree and press `m`, or right-click its card. The resizable note window has a single-line PR title, Markdown body, and live preview. Save with **Save** or `Ctrl/Cmd+S`; **Cancel** or `Esc` discards edits. **Delete note** removes both saved artifacts after confirmation. Saving an empty field removes its file.

| File under the worktree | Purpose |
|---|---|
| `.biomelab/pr-title.md` | Single-line PR/MR title |
| `.biomelab/note.md` | Markdown task description and PR/MR body |

BiomeLab excludes `.biomelab/` through Git's `info/exclude`. These files are inside the shared workspace, so sandbox agents and external tools can read or write them.

When creating a PR/MR with `Shift+P`, BiomeLab offers to use the saved title and description. You can review them first or use commit-derived defaults. Existing requests use the push-only flow; notes are not an automatic update to an existing request's text.

## Issue context and agent handoff

When you create a worktree from a GitHub issue, BiomeLab preserves the original issue requirements in `.biomelab/issue.md` and initializes `.biomelab/progress.md` with sections for completed work, decisions, remaining tasks or blockers, validation, and revision or uncommitted state. The initial template records no completed work; agents maintain it as work advances. It seeds the PR drafts with the issue heading, link, body, and title. Progress is for agents and handoff; Send PR never includes it automatically.

The new worktree also receives a marked task-context block in `AGENTS.md`, an existing `AGENTS.override.md`, `CLAUDE.md`, `GEMINI.md`, and `.kiro/steering/biomelab-task.md`. The instructions direct agents to read the issue snapshot and current progress at the start of every session, including the first, inspect the repository state, reconcile stale notes with the code, and update progress after meaningful milestones and before handoff. New instruction files are ignored by Git; existing tracked files are intentionally changed. Setup helpers preserve the original issue snapshot and existing progress on retry and upgrade exact earlier generated instruction blocks in place. BiomeLab does not infer progress or continuously reload it. The `m` editor still edits only PR drafts; agents and external editors can open the issue and progress files directly.

If note or instruction setup only partially succeeds, the worktree remains and the error names the affected artifact. The note editor can repair note content but does not repair every bootstrap failure. The built-in Docker Agent kit loads `AGENTS.md`; a manually invoked custom Docker Agent must configure its own prompt-file handoff.

## re_gent activity

With `rgt` installed on the host, BiomeLab initializes re_gent for regular-mode worktrees and installs Claude Code hooks in `.claude/settings.json`, preserving unrelated hooks. Recorded activity lives in `.regent/`, which is excluded from Git status. Detection of another agent on a card does not imply that its conversations are recorded; recording depends on re_gent and that agent's integration.

Press `l` to open the activity window. It shows recorded human prompts, agent replies, and expandable tool calls. Selecting another worktree and pressing `l` reuses the window. **Export JSON…** saves the raw log through a native save dialog, with a Fyne fallback.

Sandbox-only repositories skip host-side re_gent initialization; recording must be configured inside the sandbox, for example with a compatible regent kit. The current GUI viewer still runs the **host** `rgt log --json` in the worktree, so viewing and exporting require host `rgt` and accessible log data. The tray's **Dependencies** dialog diagnoses tools; it does not open the activity viewer.
