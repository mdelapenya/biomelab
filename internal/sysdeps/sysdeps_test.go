package sysdeps

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/mdelapenya/biomelab/internal/config"
)

func TestStatusString(t *testing.T) {
	tests := []struct {
		s    Status
		want string
	}{
		{StatusOK, "ok"},
		{StatusDegraded, "degraded"},
		{StatusMissing, "missing"},
		{StatusNA, "n/a"},
		{Status(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("Status(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestRunAll_ProbesEvenWhenAppliesFalse(t *testing.T) {
	// Probes always run, regardless of Applies. The visibility decision
	// is made later by ApplyVisibility — that way installed tools show
	// as green even when the user's config doesn't strictly need them.
	called := false
	checks := []Check{
		{
			Name:    "skip-me",
			Applies: func(*config.Config) bool { return false },
			Probe: func() Result {
				called = true
				return Result{Status: StatusOK, Version: "1.0"}
			},
		},
	}
	got := runAll(&config.Config{}, checks)
	if !called {
		t.Error("Probe must run even when Applies returns false")
	}
	if got[0].Result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK (probe truth)", got[0].Result.Status)
	}
}

func TestApplyVisibility(t *testing.T) {
	cases := []struct {
		name string
		in   []Reported
		want []string // surviving check names, in order
	}{
		{
			name: "installed tool with applies=false stays visible",
			in: []Reported{
				{
					Check:  Check{Name: "sbx", Applies: func(*config.Config) bool { return false }},
					Result: Result{Status: StatusOK, Version: "v0.29.0"},
				},
			},
			want: []string{"sbx"},
		},
		{
			name: "missing tool with applies=false is hidden",
			in: []Reported{
				{
					Check:  Check{Name: "sbx", Applies: func(*config.Config) bool { return false }},
					Result: Result{Status: StatusMissing},
				},
			},
			want: nil,
		},
		{
			name: "missing tool with applies=true stays visible",
			in: []Reported{
				{
					Check:  Check{Name: "sbx", Applies: func(*config.Config) bool { return true }},
					Result: Result{Status: StatusMissing},
				},
			},
			want: []string{"sbx"},
		},
		{
			name: "missing tool with nil Applies stays visible (Applies absent ⇒ always applies)",
			in: []Reported{
				{Check: Check{Name: "gh"}, Result: Result{Status: StatusMissing}},
			},
			want: []string{"gh"},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyVisibility(tt.in, &config.Config{})
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d", len(got), len(tt.want))
			}
			for i, w := range tt.want {
				if got[i].Check.Name != w {
					t.Errorf("got[%d] = %q, want %q", i, got[i].Check.Name, w)
				}
			}
		})
	}
}

func TestRunAll_NilApplies(t *testing.T) {
	// Applies=nil means always applies.
	checks := []Check{
		{Name: "always", Probe: func() Result { return Result{Status: StatusOK, Version: "1.0"} }},
	}
	got := runAll(&config.Config{}, checks)
	if got[0].Result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", got[0].Result.Status)
	}
	if got[0].Result.Version != "1.0" {
		t.Errorf("Version = %q, want 1.0", got[0].Result.Version)
	}
}

func TestRunAll_NilProbe(t *testing.T) {
	// A Check with no Probe should not panic; it produces a Missing result
	// with a note so the bug surfaces in the UI rather than a crash.
	checks := []Check{{Name: "no-probe"}}
	got := runAll(&config.Config{}, checks)
	if got[0].Result.Status != StatusMissing {
		t.Errorf("Status = %v, want StatusMissing", got[0].Result.Status)
	}
	if got[0].Result.Note == "" {
		t.Error("Note is empty; want explanation for missing probe")
	}
}

func TestSummarize(t *testing.T) {
	reps := []Reported{
		{Result: Result{Status: StatusOK}},
		{Result: Result{Status: StatusOK}},
		{Result: Result{Status: StatusDegraded}},
		{Result: Result{Status: StatusMissing}},
		{Result: Result{Status: StatusNA}},
	}
	c := Summarize(reps)
	if c.OK != 2 || c.Degraded != 1 || c.Missing != 1 || c.NA != 1 {
		t.Errorf("Summarize = %+v", c)
	}
	if c.Total() != 4 {
		t.Errorf("Total() = %d, want 4 (excludes N/A)", c.Total())
	}
}

func TestCache_MemoizesWithinTTL(t *testing.T) {
	var calls atomic.Int32
	checks := []Check{
		{
			Name: "counted",
			Probe: func() Result {
				calls.Add(1)
				return Result{Status: StatusOK}
			},
		},
	}
	cache := NewCache(time.Hour)
	cache.SetChecks(checks)

	cfg := &config.Config{}
	cache.Get(cfg)
	cache.Get(cfg)
	cache.Get(cfg)

	if got := calls.Load(); got != 1 {
		t.Errorf("Probe calls = %d, want 1 (memoized)", got)
	}
}

func TestCache_InvalidateForcesReProbe(t *testing.T) {
	var calls atomic.Int32
	checks := []Check{
		{Name: "x", Probe: func() Result {
			calls.Add(1)
			return Result{Status: StatusOK}
		}},
	}
	cache := NewCache(time.Hour)
	cache.SetChecks(checks)

	cfg := &config.Config{}
	cache.Get(cfg)
	cache.Invalidate()
	cache.Get(cfg)

	if got := calls.Load(); got != 2 {
		t.Errorf("Probe calls = %d, want 2 (post-invalidate re-probe)", got)
	}
}

func TestCache_ConfigPointerChangeDoesNotReprobe(t *testing.T) {
	var calls atomic.Int32
	checks := []Check{
		{Name: "x", Probe: func() Result {
			calls.Add(1)
			return Result{Status: StatusOK}
		}},
	}
	cache := NewCache(time.Hour)
	cache.SetChecks(checks)

	cfgA := &config.Config{}
	cfgB := &config.Config{}
	cache.Get(cfgA)
	cache.Get(cfgB) // same raw probes despite a fresh config allocation

	if got := calls.Load(); got != 1 {
		t.Errorf("Probe calls = %d, want 1", got)
	}
}

func TestCache_ConcurrentReadersShareProbe(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	cache := NewCache(time.Hour)
	cache.SetChecks([]Check{{Name: "x", Probe: func() Result {
		calls.Add(1)
		close(entered)
		<-release
		return Result{Status: StatusOK}
	}}})
	first := make(chan []Reported, 1)
	second := make(chan []Reported, 1)
	go func() { first <- cache.Get(&config.Config{}) }()
	<-entered
	go func() { second <- cache.Get(&config.Config{}) }()
	close(release)
	<-first
	<-second
	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent probe calls = %d, want 1", got)
	}
}

func TestCache_InvalidateDoesNotWaitForProbe(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	cache := NewCache(time.Hour)
	cache.SetChecks([]Check{{Name: "x", Probe: func() Result {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return Result{Status: StatusOK}
	}}})
	done := make(chan struct{})
	go func() { cache.Get(nil); close(done) }()
	<-entered
	invalidated := make(chan struct{})
	go func() { cache.Invalidate(); close(invalidated) }()
	select {
	case <-invalidated:
	case <-time.After(time.Second):
		t.Fatal("Invalidate blocked on a slow probe")
	}
	close(release)
	<-done
	if got := calls.Load(); got != 2 {
		t.Fatalf("probe calls after invalidation = %d, want 2", got)
	}
}

func TestApplySuppressionForRequired_MixedProviders(t *testing.T) {
	reps := []Reported{
		{Check: Check{Name: "gh", SuppressIfAny: []string{"glab"}}, Result: Result{Status: StatusMissing}},
		{Check: Check{Name: "glab", SuppressIfAny: []string{"gh"}}, Result: Result{Status: StatusOK}},
	}
	got := ApplySuppressionForRequired(reps, map[string]bool{"gh": true, "glab": true})
	if len(got) != 2 || got[0].Check.Name != "gh" {
		t.Fatalf("required GitHub CLI disappeared: %+v", got)
	}
}

func TestHasSandboxRepo(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"nil config", nil, false},
		{"no repos", &config.Config{}, false},
		{
			"regular-only",
			&config.Config{Repos: []config.RepoEntry{
				{Modes: []config.ModeEntry{{Type: "regular"}}},
			}},
			false,
		},
		{
			"one sandbox",
			&config.Config{Repos: []config.RepoEntry{
				{Modes: []config.ModeEntry{{Type: "regular"}}},
				{Modes: []config.ModeEntry{{Type: "sandbox"}}},
			}},
			true,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasSandboxRepo(tt.cfg); got != tt.want {
				t.Errorf("hasSandboxRepo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistryContents(t *testing.T) {
	want := []string{"rgt", "gh", "glab", "sbx"}
	got := Registry()
	if len(got) != len(want) {
		t.Fatalf("Registry() returned %d checks, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("Registry()[%d].Name = %q, want %q", i, got[i].Name, w)
		}
	}
}

func TestRegistry_OptionalSet(t *testing.T) {
	// Only rgt is currently Optional. Optional checks live in the
	// dialog's "Optional tools" expander while missing, and auto-promote
	// to the main list once detected.
	for _, c := range Registry() {
		if c.Name == "rgt" && !c.Optional {
			t.Error("rgt should be Optional")
		}
		if c.Name != "rgt" && c.Optional {
			t.Errorf("%s is Optional but should not be", c.Name)
		}
	}
}

func TestApplySuppression(t *testing.T) {
	cases := []struct {
		name string
		in   []Reported
		want []string // expected check names in result, in order
	}{
		{
			name: "missing glab is hidden when gh is installed",
			in: []Reported{
				{Check: Check{Name: "gh"}, Result: Result{Status: StatusOK}},
				{Check: Check{Name: "glab", SuppressIfAny: []string{"gh"}}, Result: Result{Status: StatusMissing}},
			},
			want: []string{"gh"},
		},
		{
			name: "missing gh is hidden when glab is installed",
			in: []Reported{
				{Check: Check{Name: "gh", SuppressIfAny: []string{"glab"}}, Result: Result{Status: StatusMissing}},
				{Check: Check{Name: "glab"}, Result: Result{Status: StatusOK}},
			},
			want: []string{"glab"},
		},
		{
			name: "both missing — neither is suppressed",
			in: []Reported{
				{Check: Check{Name: "gh", SuppressIfAny: []string{"glab"}}, Result: Result{Status: StatusMissing}},
				{Check: Check{Name: "glab", SuppressIfAny: []string{"gh"}}, Result: Result{Status: StatusMissing}},
			},
			want: []string{"gh", "glab"},
		},
		{
			name: "degraded counts as satisfied",
			in: []Reported{
				{Check: Check{Name: "gh"}, Result: Result{Status: StatusDegraded}},
				{Check: Check{Name: "glab", SuppressIfAny: []string{"gh"}}, Result: Result{Status: StatusMissing}},
			},
			want: []string{"gh"},
		},
		{
			name: "suppression does not affect OK entries",
			in: []Reported{
				{Check: Check{Name: "gh"}, Result: Result{Status: StatusOK}},
				{Check: Check{Name: "glab", SuppressIfAny: []string{"gh"}}, Result: Result{Status: StatusOK}},
			},
			want: []string{"gh", "glab"},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplySuppression(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d (%v)", len(got), len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if got[i].Check.Name != w {
					t.Errorf("got[%d] = %q, want %q", i, got[i].Check.Name, w)
				}
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	// Sample copied from a real `rgt version` invocation, which embeds
	// 256-color CSI sequences around the tool name. The output should
	// reduce to plain text.
	in := "\x1b[38;5;141mre_gent\x1b[0m version dev (commit: unknown)"
	want := "re_gent version dev (commit: unknown)"
	if got := stripANSI(in); got != want {
		t.Errorf("stripANSI() = %q, want %q", got, want)
	}
}

func TestPartition(t *testing.T) {
	reps := []Reported{
		{Check: Check{Name: "gh"}, Result: Result{Status: StatusOK}},
		{Check: Check{Name: "rgt", Optional: true}, Result: Result{Status: StatusMissing}},
		{Check: Check{Name: "sbx"}, Result: Result{Status: StatusDegraded}},
		{Check: Check{Name: "rgt2", Optional: true}, Result: Result{Status: StatusOK}},
	}
	primary, optional := Partition(reps)

	if len(primary) != 3 {
		t.Fatalf("primary len = %d, want 3 (gh, sbx, rgt2)", len(primary))
	}
	wantPrimary := []string{"gh", "sbx", "rgt2"}
	for i, w := range wantPrimary {
		if primary[i].Check.Name != w {
			t.Errorf("primary[%d] = %q, want %q", i, primary[i].Check.Name, w)
		}
	}
	if len(optional) != 1 || optional[0].Check.Name != "rgt" {
		t.Errorf("optional = %+v, want [rgt]", optional)
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	var calls atomic.Int32
	checks := []Check{
		{Name: "x", Probe: func() Result {
			calls.Add(1)
			return Result{Status: StatusOK}
		}},
	}
	cache := NewCache(10 * time.Millisecond)
	cache.SetChecks(checks)

	cfg := &config.Config{}
	cache.Get(cfg)
	time.Sleep(30 * time.Millisecond)
	cache.Get(cfg)

	if got := calls.Load(); got != 2 {
		t.Errorf("Probe calls = %d, want 2 (post-TTL re-probe)", got)
	}
}
