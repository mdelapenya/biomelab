# Issue #82: Create a worktree from an issue

Status: issue-worktree flow and progress-aware session handoff implemented
and validated. Full race tests, lint, and build pass on macOS (existing linker
warnings only); the original issue flow also passed independent review.

Source: https://github.com/mdelapenya/biomelab/issues/82
The issue contains only the title “Fetch an issue to create a worktree from
the issue”; there is no description or discussion. The user approved this
interpretation and added the first-session agent note-loading requirement.

Prepared from main at `4d1637b`, in branch
`feat/issue-82-worktree-from-issue` and worktree `../biomelab-issue-82`.

## Approved product contract

1. On the main card, `i` opens “Create Worktree from Issue”. Existing `c` and
   `f` actions retain their behavior. Add the action to the main-card help.
2. Initially support GitHub through the authenticated `gh` CLI. Accept a
   positive issue number for the selected repository or `owner/repo#number`.
   GitLab, Enterprise-specific handling, and URL input are deferred explicitly.
   Unsupported repository providers get a clear message, not a GitHub request.
3. Fetch issue number, title, body, canonical URL, and state asynchronously.
   Show a cancellable loading dialog, then a preview with source repository,
   title, state, issue link, body, destination repository, and editable branch.
   Empty bodies are valid (including issue #82). Closed issues may be used;
   their state remains visible in the preview.
4. Suggest `issue-<number>-<title-slug>`; use lowercase ASCII letters, digits,
   and hyphens, collapse separators, trim trailing hyphens, and cap the title
   slug at 60 characters. If the title yields no slug, use `issue-<number>`.
   For this flow, edited names must also be nonempty, at most 100 characters,
   start with an ASCII letter or digit, and contain only those same characters.
   Explain the allowed format inline. This avoids silently renaming slash
   branches through the current CreateWorktree implementation.
5. “Create” makes a new host worktree under the selected repository's existing
   `.biomelab-worktrees/` directory, using normal creation semantics: the main
   worktree's current local HEAD at creation time. Explain this in the preview.
   Do not pull, merge, change the main checkout, or fetch a PR ref. An explicitly
   qualified issue supplies context; it does not change the destination repo.
6. Save issue title to `.biomelab/pr-title.md`. Save a Markdown heading with
   issue number/title, `Source: <canonical URL>`, and the issue body to
   `.biomelab/note.md`, using existing notes APIs. These are editable task
   notes, visible inside the existing sandbox mount and excluded from git.
   Use a neutral source link; do not invent a closing keyword or PR title type.
7. Refresh the originating repository after creation. Do not automatically
   open a terminal/editor, start an agent, create a sandbox, push, open a PR,
   or modify the issue. Existing `m` and Send PR flows consume the notes.
8. User addition: when an agent first starts in the added worktree, it must
   load that worktree's notes. Creation must install a supported agent-context
   handoff, preserving existing repository instructions; merely storing notes
   or printing them in a terminal does not satisfy this requirement. Do not
   auto-launch an agent. Verify each supported handoff against official/local
   agent behavior and report unsupported cases explicitly.
   The chosen handoff is `notes.EnsureAgentBootstrap(worktreePath)`: install
   marked instructions to read task context and progress before work. Preserve and
   append to existing instruction files; tracked files consequently show a
   deliberate diff. Exclude newly created instruction files through the common
   gitdir. Repeated calls are idempotent; there is no consumed marker, so a new
   session can load updated notes. Bootstrap failures are partial-success
   errors and never cause worktree deletion.
   Implemented targets: AGENTS.md (plus an existing AGENTS.override.md),
   CLAUDE.md, GEMINI.md, and .kiro/steering/biomelab-task.md. Generated AGENTS
   guidance also preserves CLAUDE.md fallback instructions. Docker Sandboxes'
   built-in Docker Agent kit uses AGENTS.md via agentInstructions.filename;
   manually launched custom host agents require their own compatible context
   configuration. Shell mode has no model context to load.
9. User-approved follow-up: retain original issue requirements separately in
   `.biomelab/issue.md` and initialize `.biomelab/progress.md` for completed work,
   decisions, remaining work/blockers, validation results, and revision/local
   changes. The initial template records no invented progress. Existing
   `note.md` and `pr-title.md` remain editable PR drafts; progress is not sent as
   the PR description. The note editor retains its existing draft-only scope.
10. Each new session reads original issue context and current progress, then
    inspects the actual code and Git state before planning. It must reconcile
    stale notes rather than assume that the original issue is untouched. Agents
    update progress after meaningful milestones and before handing off or
    ending a session. There is no automatic summarizer, continuous note reload,
    or consumed marker. Exact earlier generated instruction blocks are upgraded
    in place without duplicating directives; unrelated guidance is preserved.

### Follow-up delegation

- Sol owns instruction changes and safe migration in `notes/bootstrap.go` and
  its tests.
- Terra owns new issue/progress artifact helpers in `notes/context.go`, their
  tests, and issue-creation integration/tests in `ops/issue.go`.
- Luna owns the corresponding README, architecture, and product-guidance edits.
- The coordinator reviews migration and artifact preservation, checks PR-draft
  separation, and runs final race tests, lint, and build. No agent may edit a
  different agent's files or expand the GUI scope without reassignment.

## Repository facts and constraints

- `internal/ops/worktree.go`: normal creation and optional regent bootstrap.
- `internal/git/worktree.go`: CreateWorktree currently replaces `/` with `-`
  in both the directory and branch. Keep the issue flow's names slash-free.
- `internal/github/pr.go`: precedent for CLI integration, but its permissive
  parser and catch-all “not found” errors must not be copied.
- `internal/notes/notes.go`: existing title/body persistence and git exclusion.
- `internal/gui/shortcuts.go`, `input_dialogs.go`, `dashboard.go`: entry points.
- `internal/gui/sandbox_setup.go`: precedent for cancellable lookup and status
  owned by the originating project. Do not copy async active-repo mistakes from
  older worktree handlers.
- Follow `CLAUDE.md`, `ARCHITECTURE.md`, and existing dialog widgets. Parts of
  `.claude/skills/product-owner/SKILL.md` describe the retired TUI. Parts of
  `.claude/skills/fyne-developer/SKILL.md` recommend manual overlay removal;
  the current code and CLAUDE.md explicitly require `Hide()` and proper dialog
  cleanup instead. Use current behavior when these sources disagree.

## Contracts to freeze before delegation

The coordinator owns these interfaces. Agents must report needed changes
before changing any agreed contract.

- `internal/github/issue.go`: `IssueRef { Number int; Repo string }`,
  `IssueInfo { Number int; Title, Body, URL, State string }`,
  `ParseIssueRef(string) (IssueRef, error)`, and
  `FetchIssue(context.Context, string, IssueRef) (IssueInfo, error)` where the
  string is the local repository directory. Keep issue support separate from
  PR parsing and the PRProvider interface.
- Invoke `gh issue view <number> --json number,title,body,url,state`, with
  `--repo` for an explicit qualified reference and `cmd.Dir` set correctly.
  Use `exec.CommandContext`, an explicit lookup timeout, structured arguments,
  strict positive-number parsing with overflow rejection, and validated
  owner/repository components. Preserve actionable CLI errors and distinguish
  cancellation, timeout, malformed JSON, and missing required response fields.
- `internal/ops/issue.go`: `SuggestedIssueBranch(github.IssueInfo) string`,
  `ValidateIssueBranch(string) error`, and
  `CreateWorktreeFromIssue(*git.Repository, github.IssueInfo, string)` returning
  a result with `BranchName`, `WtPath`, `Err`, and `NotesErr` fields.
  `Err` means creation failed; `NotesErr` means the worktree exists but its
  context could not be fully saved. Resolve and return the actual created path.
- Reuse normal creation and the existing notes APIs. An existing branch/path
  must cause a collision error without replacing anything. Never write issue
  notes into an existing worktree after failed creation. Do not overwrite
  preexisting note artifacts checked out from the base commit; report them as
  a notes conflict. Preserve a newly created worktree if note writes fail,
  report the path and failed artifact. The note editor can repair note content;
  instruction-file or filesystem failures require repairing the reported file
  or permissions and completing agent setup separately. Do not claim that the
  note editor reruns agent setup.
  Do not silently declare full success or destructively roll back.
- GUI flow: input → cancellable lookup → preview → creation → result. Cancel
  before creation has no filesystem side effects. Prevent double submission;
  once creation starts, finish reporting its outcome. Stale lookup results
  cannot reopen dialogs after cancellation. Capture the originating repo/mode
  and refresh manager; completion must not update an unrelated active repo.

## Subagent execution plan

The coordinator owns scope, interfaces, sequencing, integration, and final
review. Each implementation subagent receives this file plus a bounded task.
User-selected models: Sol for hard tasks, Terra for medium tasks, Luna for
mechanical/easy tasks. Sol owns operations, agent-context integration, complex
GUI lifecycle work, and adversarial review; Terra owns bounded GitHub lookup;
Luna owns documentation updates after behavior is finalized.

| Order | Assignment | Owned files | Completion gate |
| --- | --- | --- | --- |
| 0 | Coordinator freezes contracts and records baseline test outcome | This plan | Resolve any contract blocker before code changes |
| 1A | GitHub issue lookup and parsing | `internal/github/issue.go`, `issue_test.go` | Parser and fake-CLI tests pass; no live GitHub dependency |
| 1B | Worktree orchestration and note seeding | `internal/ops/issue.go`, `issue_test.go` | Temp-repo tests prove creation, collisions, notes, and partial failures |
| 2 | GUI workflow after 1A/1B integration | New issue dialog/controller/tests; minimal edits to `shortcuts.go`, `dashboard.go`, and `app.go` if needed | Workflow tests pass, cancellation and repo ownership verified |
| 3A | Documentation after GUI behavior settles | `README.md`, `ARCHITECTURE.md`, relevant new product-owner feature/shortcut entries | Docs match implemented behavior and limitations |
| 3B | Independent read-only review | No file edits | Review diff against contract and failure cases; coordinator assigns fixes |
| 4 | Coordinator integrates and validates | Cross-cutting fixes only after ownership handoff | All checks below pass or blockers are reported explicitly |

1A and 1B may run concurrently after the coordinator makes the agreed issue
types available; they must not create competing definitions. GUI work starts
after backend contracts and behavior are reviewed. Documentation and final
review can run concurrently within the available agent slots.

### Rules for every subagent

1. Work only in the issue worktree and assigned files. Read this plan and
   repository guidance first. Do not edit main or undo another agent's work.
2. Implement the assigned contract exactly. Raise ambiguity, new dependencies,
   scope changes, and shared-file edits to the coordinator before proceeding.
3. Do not spawn further agents, commit, push, publish, or contact GitHub users.
4. Use existing patterns, no unrelated refactors or dependency upgrades. Keep
   GitHub text as data; never interpolate it into shell commands or paths.
5. Keep network/git work off the UI thread and UI state changes on `fyne.Do`.
   Use existing focus-aware dialog widgets, Escape cleanup, and `Hide()`.
6. Test observable behavior with temporary repositories and injectable CLI or
   operation boundaries. No tests requiring real authentication or Docker.
   Do not add mutable global test hooks that race with concurrent tests.
7. Report changed files, tests and outcomes, contract compliance, remaining
   risks, and any blocked cases. The coordinator reviews before advancing.

## Acceptance and validation

- Parser: valid numeric/qualified references, whitespace, zero/negative and
  overflowing numbers, malformed repository components, unsupported formats.
- Lookup: exact args/cwd, empty body, closed issue, unavailable CLI, auth or
  network failure, timeout/cancel, invalid JSON and missing essential fields.
- Branch generation: punctuation, Unicode-only/empty titles, long titles,
  safe fallback; edited-name validation rejects traversal and separators.
- Temp-repo integration: correct base commit, branch/path, issue notes and
  source URL, ignored note files, unchanged main checkout, duplicate branch
  and path preservation, notes conflict and write-failure reporting.
- GUI: main-card gating, provider gating, regular/sandbox modes, preview and
  validation, Escape from focused inputs, loading cancel with late completion,
  repeated Create, repo switch/removal, and originating-repo refresh.
- Regression: normal creation and Fetch PR still work; task-note editor reads
  generated files and Send PR can offer them using the existing opt-in flow.
- Coordinator runs focused package tests during integration, then the required
  `go test -race ./...` (or `task test-race` with configured toolchain),
  `task lint`, and `task build`. Record failures and determine whether they
  predate this work; do not suppress checks to claim completion.
- Manually exercise lookup/preview/cancel and successful creation in a disposable
  registered repository. Check regular and sandbox-mode presentation; report
  Docker-dependent checks as unverified if Docker is unavailable.

## Scope decisions open to user refinement

The defaults are GitHub-first, shortcut `i`, local HEAD as the base, the two
reference formats above, and automatic issue-context notes. GitLab parity,
URL input, a base-branch picker, issue comments/attachments, and launching an
agent are separate extensions. Agents must not add them without a coordinator
plan revision.

## Implementation evidence

- Progress-aware follow-up: Sol implemented startup guidance and exact legacy
  block upgrades (including CRLF and permission preservation); Terra added
  issue/progress storage and four-artifact collision checks; Luna updated docs.
  Coordinator reviewed artifact preservation, migration, and PR-draft separation.
- Follow-up validation passed: `go test -race ./...`,
  `task lint GOROOT_GVM= GO=go` (zero issues), and
  `task build GOROOT_GVM= GO=go`. Tests cover preserving edited progress and
  original requirements on retry, unsafe targets, artifact conflicts, and
  repeated instruction migration. Live agent sessions remain untested.
- Baseline full race suite passed before edits.
- GitHub parser/lookup tests and backend git/notes/ops race tests passed.
- Live smoke test fetched issue #82 through the new lookup API, created an
  issue worktree in a disposable repository, verified source notes and agent
  instruction files, and confirmed duplicate creation is rejected. The fixture
  repository was removed afterwards.
- Review corrections include instruction setup idempotence, preservation of
  existing instruction precedence, early worktree collision checks, support
  for separate Git metadata directories, and GUI callback/modal ownership.
- Final `go test -race ./...` passed; the independent reviewer also ran an
  uncached `go test -race -count=1 ./internal/gui` successfully.
- `task lint GOROOT_GVM= GO=go` passed with zero issues. `task build` passed;
  the final source fingerprint was rechecked with the installed Go toolchain.
  The override avoided waiting on gvm toolchain discovery; project dependencies
  and tool versions were not changed.
- Headless GUI tests cover the complete issue flow in regular and sandbox
  modes, cancellation, ownership, errors, and long-content bounds. A rendered
  preview was visually inspected at `/tmp/biomelab-issue82-preview.png`.
- Native macOS desktop testing exercised numeric and qualified lookup against
  issue #82, cancellation, invalid references and branch names, creation,
  duplicate protection, and editing/saving PR drafts. Filesystem checks verified
  the snapshot, progress template, startup instructions, base commit, and
  preservation of the snapshot/progress while editing a draft.
- Desktop testing exposed a linked-worktree dirty-indicator bug: native Git
  honors the shared instruction-file exclusions while go-git's linked status
  misses them. Sol corrected the bootstrap-file status filtering, preserving
  tracked changes and ignore-rule negations/precedence. Terra reviewed it.
  The rebuilt native app now shows the linked worktree clean, matching Git.
  Final full race tests, lint (zero issues), and build passed again.
  Screenshot: `/tmp/biomelab-desktop-82-fixed-grid.png`; detailed desktop results:
  `/tmp/biomelab-desktop-82-results.json`. The disposable repository and its
  registration were removed after testing; the user's original app was not
  terminated. No GitHub writes or Docker/model launches occurred.
- Live Docker/model sessions were not exercised. Agent handoff behavior is
  based on supported instruction-file contracts and tested generated artifacts,
  not a claim that a live model was launched.
- Independent review reports no remaining concrete blockers. Main remains
  clean; implementation is on the separate issue branch.
