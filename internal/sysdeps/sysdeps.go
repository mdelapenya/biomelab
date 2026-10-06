// Package sysdeps inspects external CLI dependencies biomelab relies on
// (gh, glab, sbx, docker, rgt) and reports their availability with degraded
// states surfaced (auth failures, daemon down, etc.).
//
// Each check is a Check entry in a registry: a Probe function returns a
// Result describing what was found. Probes shell out via os/exec, so the
// package is the centralized home for that pattern. The GUI consumes Reported
// results through a Cache to avoid re-probing on every menu render.
package sysdeps

import (
	"sync"
	"time"

	"github.com/mdelapenya/biomelab/internal/config"
)

// Status describes the readiness of a single dependency.
type Status int

const (
	// StatusNA means the dependency is not relevant to the user's current
	// configuration (e.g. sbx when no sandbox-mode repo is registered).
	StatusNA Status = iota
	// StatusOK means the dependency is installed and usable.
	StatusOK
	// StatusDegraded means the dependency is installed but not usable
	// (e.g. gh present but not authenticated, sbx daemon not running).
	StatusDegraded
	// StatusMissing means the dependency is not in PATH.
	StatusMissing
)

// String returns a short label suitable for logs and tests.
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusDegraded:
		return "degraded"
	case StatusMissing:
		return "missing"
	case StatusNA:
		return "n/a"
	default:
		return "unknown"
	}
}

// Result describes the outcome of probing a single dependency.
type Result struct {
	Status  Status
	Version string // best-effort; "" when not detected
	Note    string // human-readable detail, e.g. "not authenticated"
}

// Check describes a single dependency biomelab knows how to inspect.
type Check struct {
	// Name is the binary name (e.g. "rgt"). Kept lowercase, single token.
	Name string
	// DisplayName is the user-facing tool name (e.g. "re_gent").
	DisplayName string
	// Reason explains what biomelab uses the tool for; shown in the dialog.
	Reason string
	// Applies returns false when the tool is not relevant for the user's
	// current configuration. A nil Applies is treated as "always applies".
	Applies func(*config.Config) bool
	// Probe runs the actual check. Required.
	Probe func() Result
	// InstallHint is a short, copy-pasteable hint shown in the dialog when
	// the tool is missing (e.g. "brew install gh"). Empty allowed.
	InstallHint string
	// DocsURL is the install-docs page for the tool (empty allowed).
	DocsURL string
	// Optional marks the check as opt-in: it lives in a collapsed
	// "Optional tools" section of the dialog while missing, and is
	// auto-promoted to the main list once installation is detected.
	// Use for features the user must actively opt into (e.g. rgt).
	Optional bool
	// SuppressIfAny lists Check names that, when satisfied (StatusOK or
	// StatusDegraded), cause this Check's Missing result to be filtered
	// from the displayed set. Use for interchangeable tools where any one
	// is enough (e.g. gh and glab — users on GitHub don't need glab).
	// Has no effect when this check itself is OK/Degraded/NA.
	SuppressIfAny []string
}

// Reported pairs a Check with its Probe Result.
type Reported struct {
	Check  Check
	Result Result
}

// runAll executes the given checks. Probes always run so installed tools
// are recognized as green even when the user's config doesn't strictly
// "need" them (e.g. sbx installed but no sandbox-mode repo configured).
// The Applies callback is consulted by ApplyVisibility, not here, so
// Result.Status reflects the on-machine truth.
func runAll(cfg *config.Config, checks []Check) []Reported {
	out := make([]Reported, len(checks))
	for i, c := range checks {
		if c.Probe == nil {
			out[i] = Reported{Check: c, Result: Result{Status: StatusMissing, Note: "no probe defined"}}
			continue
		}
		out[i] = Reported{Check: c, Result: c.Probe()}
	}
	// ApplyVisibility uses cfg later; probes themselves are config independent.
	_ = cfg
	return out
}

// RunAll runs the default registry against cfg.
func RunAll(cfg *config.Config) []Reported {
	return runAll(cfg, Registry())
}

// ApplyVisibility hides Missing entries whose Applies callback reports that
// the tool is not relevant to the user's current configuration (e.g. sbx
// when no repo uses sandbox mode). Installed tools (OK/Degraded) always
// show — they're informational, never noise. Returns a new slice.
func ApplyVisibility(reps []Reported, cfg *config.Config) []Reported {
	out := make([]Reported, 0, len(reps))
	for _, r := range reps {
		if r.Result.Status == StatusMissing && r.Check.Applies != nil && !r.Check.Applies(cfg) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Counts summarizes a slice of Reported.
type Counts struct {
	OK       int
	Degraded int
	Missing  int
	NA       int
}

// Total is OK + Degraded + Missing (i.e. not N/A) — the denominator for the
// "Dependencies: N/M ✓" systray summary.
func (c Counts) Total() int { return c.OK + c.Degraded + c.Missing }

// Partition splits Reported into a primary list and an optional list. An
// Optional check that is currently Missing belongs in optional; everything
// else (including an Optional check that the user has now installed) goes
// in primary. Order within each list matches the input order.
func Partition(reps []Reported) (primary, optional []Reported) {
	for _, r := range reps {
		if r.Check.Optional && r.Result.Status == StatusMissing {
			optional = append(optional, r)
			continue
		}
		primary = append(primary, r)
	}
	return primary, optional
}

// ApplySuppression filters out Missing entries whose SuppressIfAny list
// contains the name of another check that is currently OK or Degraded.
// Used to hide redundant CLIs — e.g. glab Missing is hidden when gh is
// installed, since users on one provider don't need the other. Returns
// a new slice; the input is not mutated.
func ApplySuppression(reps []Reported) []Reported {
	return ApplySuppressionForRequired(reps, nil)
}

// ApplySuppressionForRequired keeps a missing CLI visible when a registered
// repository needs it, even if the other provider's CLI is installed.
func ApplySuppressionForRequired(reps []Reported, required map[string]bool) []Reported {
	satisfied := make(map[string]bool, len(reps))
	for _, r := range reps {
		if r.Result.Status == StatusOK || r.Result.Status == StatusDegraded {
			satisfied[r.Check.Name] = true
		}
	}
	out := make([]Reported, 0, len(reps))
	for _, r := range reps {
		if r.Result.Status == StatusMissing && !required[r.Check.Name] && hasSatisfied(r.Check.SuppressIfAny, satisfied) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func hasSatisfied(names []string, satisfied map[string]bool) bool {
	for _, n := range names {
		if satisfied[n] {
			return true
		}
	}
	return false
}

// Summarize tallies a slice of Reported.
func Summarize(reps []Reported) Counts {
	var c Counts
	for _, r := range reps {
		switch r.Result.Status {
		case StatusOK:
			c.OK++
		case StatusDegraded:
			c.Degraded++
		case StatusMissing:
			c.Missing++
		case StatusNA:
			c.NA++
		}
	}
	return c
}

// Cache memoizes Probe results for a fixed TTL so opening the systray menu
// or the dialog doesn't re-exec on every interaction. Probe results do not
// depend on Config; visibility is applied to each snapshot after reading it.
type Cache struct {
	mu     sync.Mutex
	ttl    time.Duration
	last   time.Time
	data   []Reported
	checks []Check // optional override for tests
	epoch  uint64
	flight chan struct{}
}

// NewCache returns a Cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{ttl: ttl}
}

// Get returns memoized raw results, refreshing after the TTL. Concurrent
// callers share one probe. Config is retained for source compatibility only.
func (c *Cache) Get(cfg *config.Config) []Reported {
	for {
		c.mu.Lock()
		if c.data != nil && time.Since(c.last) < c.ttl {
			data := c.data
			c.mu.Unlock()
			return data
		}
		if c.flight != nil {
			flight := c.flight
			c.mu.Unlock()
			<-flight
			continue
		}
		flight := make(chan struct{})
		c.flight = flight
		epoch := c.epoch
		checks := c.checks
		if checks == nil {
			checks = Registry()
		}
		c.mu.Unlock()

		data := runAll(cfg, checks)
		c.mu.Lock()
		current := c.epoch == epoch
		if current {
			c.data = data
			c.last = time.Now()
		}
		c.flight = nil
		close(flight)
		c.mu.Unlock()
		if current {
			return data
		}
		// An invalidation during the probe requires another pass.
	}
}

// Peek returns the last probe without running a command. The boolean reports
// whether results exist, including expired results suitable for a placeholder.
func (c *Cache) Peek() ([]Reported, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data, c.data != nil
}

// Invalidate forces the next Get to re-probe.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = nil
	c.epoch++
}

// SetChecks substitutes the registry used by Get. Intended for tests.
func (c *Cache) SetChecks(checks []Check) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks = checks
	c.data = nil
	c.epoch++
}
