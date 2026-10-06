package kits

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type failingRoundTripper func(*http.Request) (*http.Response, error)

func (f failingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCatalogSnapshotWarmReadAndFailedRefresh(t *testing.T) {
	specs := map[string]string{
		"kiro-kit":        "kind: sandbox\nname: kiro\n",
		"code-server-kit": "kind: mixin\nname: code-server\n",
		"future-kit":      "kind: future\nname: future\n",
		"new-kit":         "kind: mixin\nname: new\n",
	}
	var state atomic.Int32
	server := kitFixtureServer(t, specs, func(w http.ResponseWriter, r *http.Request) bool {
		if state.Load() == 1 && r.URL.Path == "/v2/repositories/sbx/" {
			if _, err := fmt.Fprint(w, `{"next":null,"results":[{"name":"kiro-kit","content_types":["sbx_kit"]},{"name":"new-kit","content_types":["sbx_kit"]}]}`); err != nil {
				t.Errorf("write catalog fixture: %v", err)
			}
			return true
		}
		if state.Load() == 1 && r.URL.Path == "/v2/sbx/new-kit/manifests/latest" {
			http.Error(w, "metadata unavailable", http.StatusServiceUnavailable)
			return true
		}
		if state.Load() == 2 {
			http.Error(w, "catalog unavailable", http.StatusServiceUnavailable)
			return true
		}
		return false
	})
	defer server.Close()
	path := filepath.Join(t.TempDir(), "catalog-snapshot.json")
	now := time.Now()
	sandboxes, mixins, err := fetchAvailableWithSnapshot(context.Background(), server.Client(), server.URL, server.URL, server.URL, path, now)
	if err != nil || len(sandboxes) != 1 || len(mixins) != 1 {
		t.Fatalf("initial snapshot: sandboxes=%v mixins=%v err=%v", sandboxes, mixins, err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// A fresh complete catalog must require no HTTP, including no token calls.
	noHTTP := &http.Client{Transport: failingRoundTripper(func(*http.Request) (*http.Response, error) {
		t.Error("warm snapshot issued HTTP")
		return nil, errors.New("unexpected HTTP")
	})}
	warmBases, warmMixins, err := fetchAvailableWithSnapshot(context.Background(), noHTTP, server.URL, server.URL, server.URL, path, now.Add(time.Minute))
	if err != nil || len(warmBases) != 1 || len(warmMixins) != 1 || warmBases[0].OCIReference() != sandboxes[0].OCIReference() {
		t.Fatalf("warm snapshot: sandboxes=%v mixins=%v err=%v", warmBases, warmMixins, err)
	}

	// A new Hub kit appears, but its registry metadata is unavailable. The
	// forced refresh must leave the old complete snapshot byte-for-byte intact.
	state.Store(1)
	if err := refreshCacheWithClient(context.Background(), server.Client(), server.URL, server.URL, server.URL, path); err == nil || !strings.Contains(err.Error(), "new-kit") {
		t.Fatalf("failed refresh error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed refresh replaced last successful snapshot")
	}

	// An expired snapshot remains usable when the catalog service fails.
	if err := writeSnapshot(path, sandboxes, mixins, now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	state.Store(2)
	staleBases, staleMixins, err := fetchAvailableWithSnapshot(context.Background(), server.Client(), server.URL, server.URL, server.URL, path, now)
	if err != nil || len(staleBases) != 1 || len(staleMixins) != 1 {
		t.Fatalf("stale fallback: sandboxes=%v mixins=%v err=%v", staleBases, staleMixins, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	missingBases, missingMixins, err := fetchAvailableWithSnapshot(context.Background(), server.Client(), server.URL, server.URL, server.URL, path, now)
	if err == nil || len(missingBases) != 0 || len(missingMixins) != 0 || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("missing snapshot error: sandboxes=%v mixins=%v err=%v", missingBases, missingMixins, err)
	}
}

func TestCatalogSnapshotRejectsCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog-snapshot.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"saved_at":"2026-10-05T00:00:00Z","sandboxes":[{"Name":"bad","Kind":"mixin","Reference":"docker.io/sbx/bad-kit:latest"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSnapshot(path); err == nil {
		t.Fatal("corrupt kind was accepted")
	}
}

func TestCatalogSnapshotHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog-snapshot.json")
	if err := writeSnapshot(path, []Kit{{Name: "kiro", Kind: KindSandbox, Reference: "docker.io/sbx/kiro-kit:latest"}}, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := fetchAvailableWithSnapshot(ctx, nil, "", "", "", path, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled warm read returned %v", err)
	}
}

func TestCatalogSnapshotDeadlineUsesValidatedStaleView(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog-snapshot.json")
	if err := writeSnapshot(path, []Kit{{Name: "kiro", Kind: KindSandbox, Reference: "docker.io/sbx/kiro-kit:latest"}}, nil, time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	server := kitFixtureServer(t, nil, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/v2/repositories/sbx/" {
			<-r.Context().Done()
			return true
		}
		return false
	})
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	bases, mixins, err := fetchAvailableWithSnapshot(ctx, server.Client(), server.URL, server.URL, server.URL, path, time.Now())
	if err != nil || len(bases) != 1 || len(mixins) != 0 || bases[0].Name != "kiro" {
		t.Fatalf("stale deadline fallback: bases=%v mixins=%v err=%v", bases, mixins, err)
	}
}

func TestCatalogSnapshotCancellationAndMissingSnapshotTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog-snapshot.json")
	if err := writeSnapshot(path, []Kit{{Name: "kiro", Kind: KindSandbox, Reference: "docker.io/sbx/kiro-kit:latest"}}, nil, time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 1)
	server := kitFixtureServer(t, nil, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/v2/repositories/sbx/" {
			select {
			case started <- struct{}{}:
			default:
			}
			<-r.Context().Done()
			return true
		}
		return false
	})
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := fetchAvailableWithSnapshot(ctx, server.Client(), server.URL, server.URL, server.URL, path, time.Now())
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("explicit cancellation returned %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer deadlineCancel()
	bases, mixins, err := fetchAvailableWithSnapshot(deadlineCtx, server.Client(), server.URL, server.URL, server.URL, path, time.Now())
	if !errors.Is(err, context.DeadlineExceeded) || len(bases) != 0 || len(mixins) != 0 {
		t.Fatalf("missing snapshot timeout: bases=%v mixins=%v err=%v", bases, mixins, err)
	}
}
