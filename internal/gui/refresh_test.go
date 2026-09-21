package gui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/provider"
)

func receiveRefresh[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for refresh")
		var zero T
		return zero
	}
}

func waitRefreshIdle(t *testing.T, rm *RefreshManager) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rm.mu.Lock()
		idle := true
		for _, lane := range rm.lanes {
			idle = idle && !lane.running
		}
		rm.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("refresh lanes did not drain")
}

func TestRefreshCoalescesBurstsWithoutBlockingLocal(t *testing.T) {
	started := make(chan struct{}, 10)
	local := make(chan struct{}, 10)
	release := make(chan struct{})
	var calls atomic.Int32
	rm := NewRefreshManager(nil, nil, nil, nil, nil, nil, time.Hour)
	rm.work = func(ctx context.Context, kind refreshKind, _ refreshInputs) ops.RefreshResult {
		if kind == networkRefresh {
			calls.Add(1)
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
		if kind == localRefresh {
			local <- struct{}{}
		}
		return ops.RefreshResult{}
	}
	t.Cleanup(rm.Stop)
	rm.Start()
	receiveRefresh(t, started)
	for range 100 {
		rm.TriggerNetwork()
	}
	rm.TriggerLocal()
	receiveRefresh(t, local)
	if calls.Load() != 1 {
		t.Fatal("network requests overlapped")
	}
	release <- struct{}{}
	receiveRefresh(t, started)
	release <- struct{}{}
	waitRefreshIdle(t, rm)
	if calls.Load() != 2 {
		t.Fatalf("got %d network calls, want active + one pending", calls.Load())
	}
}

func TestRefreshRunCancellationAndStaleDelivery(t *testing.T) {
	started := make(chan context.Context, 10)
	release := make(chan struct{})
	queued := make(chan uint64, 10)
	var networkResults atomic.Int32
	rm := NewRefreshManager(nil, nil, nil, nil, nil, nil, time.Hour)
	rm.work = func(ctx context.Context, kind refreshKind, _ refreshInputs) ops.RefreshResult {
		if kind == networkRefresh {
			started <- ctx
			// Model an operation that has not yet honored cancellation. A new
			// generation must not create another overlapping network worker.
			<-release
			return ops.RefreshResult{HasPRs: true}
		}
		return ops.RefreshResult{}
	}
	rm.OnRefresh = func(result ops.RefreshResult, generation uint64) {
		if result.HasPRs {
			networkResults.Add(1)
		} else {
			queued <- generation
		}
	}
	t.Cleanup(func() { rm.Stop(); close(release) })
	rm.Start()
	oldCtx := receiveRefresh(t, started)
	oldGeneration := receiveRefresh(t, queued)
	rm.Pause()
	if oldCtx.Err() != context.Canceled {
		t.Fatal("pause did not cancel work")
	}
	rm.TriggerQuick()
	rm.TriggerLocal()
	rm.TriggerNetwork()
	rm.mu.Lock()
	for _, lane := range rm.lanes {
		if lane.pending != nil {
			t.Error("paused manager accepted work")
		}
	}
	rm.mu.Unlock()
	rm.Resume()
	newGeneration := receiveRefresh(t, queued)
	// Equivalent to checking inside an already queued fyne.Do closure.
	if rm.IsCurrent(oldGeneration) || !rm.IsCurrent(newGeneration) {
		t.Fatal("queued run validity is wrong")
	}
	select {
	case <-started:
		t.Fatal("resume overlapped old network operation")
	default:
	}
	release <- struct{}{}
	newCtx := receiveRefresh(t, started)
	rm.Stop()
	if newCtx.Err() != context.Canceled {
		t.Fatal("stop did not cancel new work")
	}
	release <- struct{}{}
	waitRefreshIdle(t, rm)
	rm.Start()
	rm.Resume()
	rm.TriggerNetwork()
	rm.TriggerLocal()
	rm.TriggerQuick()
	if rm.IsCurrent(newGeneration) || networkResults.Load() != 0 {
		t.Fatal("obsolete result was accepted")
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if rm.run != nil {
		t.Fatal("stopped manager restarted")
	}
	for _, lane := range rm.lanes {
		if lane.running || lane.pending != nil {
			t.Error("stopped manager scheduled work")
		}
	}
}

func TestRefreshCachesCLIAndCopiesCandidates(t *testing.T) {
	var checks atomic.Int32
	inputs := make(chan refreshInputs, 10)
	rm := NewRefreshManager(nil, nil, nil, nil, nil, nil, time.Hour)
	rm.checkCLI = func(context.Context) provider.CLIAvailability { checks.Add(1); return provider.CLIAvailable }
	rm.work = func(_ context.Context, kind refreshKind, in refreshInputs) ops.RefreshResult {
		if kind == networkRefresh {
			inputs <- in
		}
		return ops.RefreshResult{}
	}
	t.Cleanup(rm.Stop)
	candidates := []string{"original"}
	rm.SetSandboxCandidates(candidates)
	candidates[0] = "mutated"
	rm.Start()
	first := receiveRefresh(t, inputs)
	waitRefreshIdle(t, rm)
	rm.Resume()
	second := receiveRefresh(t, inputs)
	waitRefreshIdle(t, rm)
	if checks.Load() != 1 {
		t.Fatalf("CLI checked %d times across resume", checks.Load())
	}
	for _, in := range []refreshInputs{first, second} {
		if in.candidates[0] != "original" || in.cliAvail != provider.CLIAvailable {
			t.Fatalf("wrong inputs: %+v", in)
		}
	}
}
