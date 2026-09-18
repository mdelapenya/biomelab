# Known limitations

These are implementation follow-ups identified during the documentation audit, not promised functionality.

- **Kanban keyboard navigation:** five columns are rendered, but `navigateKanbanRight` in `internal/gui/shortcuts.go` still bounds its search with `c < 4`. The final PR Merged column cannot be reached by moving right from another column. Select a card with the mouse instead. Fix the bound and add navigation coverage in a separate code change.
- **Nix packaging:** `flake.nix` has a placeholder dependency hash and needs its Fyne build/runtime dependencies validated. It is not advertised as an installation method until verified.
- **GitLab checkout:** status and creation support GitLab, but fetch-to-worktree uses GitHub helpers. Extend the provider abstraction before advertising MR checkout.
- **Issue worktree inputs:** issue-created worktrees currently accept only a positive GitHub issue number or `owner/repo#number`. GitHub issue URLs, GitLab issues, and base-branch selection are not supported.
- **Windows terminal launch:** packaged Windows builds exist; automatic terminal launch/activation is not implemented there. Custom launch currently assumes `-e sh -c`.

These limits should be removed from the guides only after the corresponding behavior has been implemented and checked.
