# Notes, PR drafts, and agent activity

## Task notes and PR drafts

Select a worktree and press `m`, or right-click its card. The resizable note window has a single-line PR title, Markdown body, and live preview. Save with **Save** or `Ctrl/Cmd+S`; **Cancel** or `Esc` discards edits. **Delete note** removes both saved artifacts after confirmation. Saving an empty field removes its file.

| File under the worktree | Purpose |
|---|---|
| `.biomelab/pr-title.md` | Single-line PR/MR title |
| `.biomelab/note.md` | Markdown task description and PR/MR body |

BiomeLab excludes `.biomelab/` through Git's `info/exclude`. These files are inside the shared workspace, so sandbox agents and external tools can read or write them.

When creating a PR/MR with `Shift+P`, BiomeLab offers to use the saved title and description. You can review them first or use commit-derived defaults. Existing requests use the push-only flow; notes are not an automatic update to an existing request's text.

## re_gent activity

With `rgt` installed on the host, BiomeLab initializes re_gent for regular-mode worktrees and installs Claude Code hooks in `.claude/settings.json`, preserving unrelated hooks. Recorded activity lives in `.regent/`, which is excluded from Git status. Detection of another agent on a card does not imply that its conversations are recorded; recording depends on re_gent and that agent's integration.

Press `l` to open the activity window. It shows recorded human prompts, agent replies, and expandable tool calls. Selecting another worktree and pressing `l` reuses the window. **Export JSON…** saves the raw log through a native save dialog, with a Fyne fallback.

Sandbox-only repositories skip host-side re_gent initialization; recording must be configured inside the sandbox, for example with a compatible regent kit. The current GUI viewer still runs the **host** `rgt log --json` in the worktree, so viewing and exporting require host `rgt` and accessible log data. The tray's **Dependencies** dialog diagnoses tools; it does not open the activity viewer.
