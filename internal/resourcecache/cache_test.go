package resourcecache

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	catalogURL = "https://hub.docker.com/v2/repositories/sbx/?page_size=100"
	mediaURL   = "https://hub.docker.com/api/media/repos_logo/v1/sbx%2Fopenclaw-kit?type=logo"
	logoURL    = "https://djeqr6to3dedg.cloudfront.net/repo-logos/sbx/openclaw-kit/live/logo-123.svg"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func reply(status int, body string, headers http.Header) *http.Response {
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body))}
}

func read(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDiskReuseExpiryAndRefresh(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		n := calls.Add(1)
		return reply(http.StatusOK, string(rune('0'+n)), http.Header{"Content-Type": {"application/json"}}), nil
	})
	opts := Options{Dir: dir, Now: func() time.Time { return now }, Transport: base}
	client := NewClientWithOptions(time.Second, opts)
	if got := read(t, client, catalogURL); got != "1" {
		t.Fatalf("first body = %q", got)
	}
	if got := read(t, NewClientWithOptions(time.Second, opts), catalogURL); got != "1" || calls.Load() != 1 {
		t.Fatalf("disk reuse: body = %q, calls = %d", got, calls.Load())
	}
	now = now.Add(59 * time.Minute)
	if got := read(t, client, catalogURL); got != "1" {
		t.Fatalf("fresh body = %q", got)
	}
	refresh := NewRefreshClientWithOptions(time.Second, opts)
	if got := read(t, refresh, catalogURL); got != "2" {
		t.Fatalf("refresh body = %q", got)
	}
	if got := read(t, client, catalogURL); got != "2" || calls.Load() != 2 {
		t.Fatalf("refreshed disk reuse: body = %q, calls = %d", got, calls.Load())
	}
	now = now.Add(time.Hour)
	if got := read(t, client, catalogURL); got != "3" || calls.Load() != 3 {
		t.Fatalf("expired body = %q, calls = %d", got, calls.Load())
	}
}

func TestCorruptAndUnwritableCacheFallsBack(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return reply(http.StatusOK, "good", nil), nil
	})
	client := NewClientWithOptions(time.Second, Options{Dir: dir, Transport: base})
	if got := read(t, client, catalogURL); got != "good" {
		t.Fatal(got)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("cache files = %v, err = %v", files, err)
	}
	path := filepath.Join(dir, files[0].Name())
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := read(t, client, catalogURL); got != "good" || calls.Load() != 2 {
		t.Fatalf("corrupt fallback: body = %q, calls = %d", got, calls.Load())
	}
	// A regular file cannot host cache entries, even as root.
	unwritable := NewClientWithOptions(time.Second, Options{Dir: path, Transport: base})
	if got := read(t, unwritable, catalogURL); got != "good" || calls.Load() != 3 {
		t.Fatalf("unwritable fallback: body = %q, calls = %d", got, calls.Load())
	}
}

func TestLogoRedirectAndSVGReused(t *testing.T) {
	for _, redirect := range []int{http.StatusMovedPermanently, http.StatusFound} {
		t.Run(http.StatusText(redirect), func(t *testing.T) {
			var calls atomic.Int32
			svg := `<svg xmlns="http://www.w3.org/2000/svg"><circle r="5"/></svg>`
			base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				switch req.URL.String() {
				case mediaURL:
					return reply(redirect, "", http.Header{"Location": {logoURL}, "Set-Cookie": {"__cf_bm=secret"}}), nil
				case logoURL:
					return reply(http.StatusOK, svg, http.Header{"Content-Type": {"image/svg+xml"}}), nil
				default:
					return nil, errors.New("unexpected URL: " + req.URL.String())
				}
			})
			opts := Options{Dir: t.TempDir(), Transport: base}
			if got := read(t, NewClientWithOptions(time.Second, opts), mediaURL); got != svg {
				t.Fatalf("SVG = %q", got)
			}
			if got := read(t, NewClientWithOptions(time.Second, opts), mediaURL); got != svg || calls.Load() != 2 {
				t.Fatalf("disk redirect/SVG: body = %q, calls = %d", got, calls.Load())
			}
		})
	}
}

func TestPNGLogoRedirectDiskReuseAndRefresh(t *testing.T) {
	const pngURL = "https://djeqr6to3dedg.cloudfront.net/repo-logos/sbx/trivy-kit/live/logo-1790074145819.png"
	makePNG := func(c color.RGBA) string {
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		img.Set(0, 0, c)
		var out bytes.Buffer
		if err := png.Encode(&out, img); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	oldPNG := makePNG(color.RGBA{R: 255, A: 255})
	newPNG := makePNG(color.RGBA{B: 255, A: 255})
	var generation, calls atomic.Int32
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		switch req.URL.String() {
		case mediaURL:
			return reply(http.StatusFound, "", http.Header{"Location": {pngURL}}), nil
		case pngURL:
			body := oldPNG
			if generation.Load() == 1 {
				body = newPNG
			}
			return reply(http.StatusOK, body, http.Header{"Content-Type": {"image/png"}}), nil
		default:
			return nil, errors.New("unexpected URL: " + req.URL.String())
		}
	})
	opts := Options{Dir: t.TempDir(), Transport: base}
	if got := read(t, NewClientWithOptions(time.Second, opts), mediaURL); got != oldPNG {
		t.Fatal("initial PNG differs")
	}
	if got := read(t, NewClientWithOptions(time.Second, opts), mediaURL); got != oldPNG || calls.Load() != 2 {
		t.Fatalf("disk PNG reuse: calls = %d, matching = %t", calls.Load(), got == oldPNG)
	}
	generation.Store(1)
	if got := read(t, NewRefreshClientWithOptions(time.Second, opts), mediaURL); got != newPNG || calls.Load() != 4 {
		t.Fatalf("PNG refresh: calls = %d, matching = %t", calls.Load(), got == newPNG)
	}
	if got := read(t, NewClientWithOptions(time.Second, opts), mediaURL); got != newPNG || calls.Load() != 4 {
		t.Fatalf("refreshed PNG reuse: calls = %d, matching = %t", calls.Load(), got == newPNG)
	}
	if _, err := png.Decode(bytes.NewReader([]byte(newPNG))); err != nil {
		t.Fatalf("invalid PNG bytes: %v", err)
	}
	files, err := os.ReadDir(opts.Dir)
	if err != nil || len(files) != 2 {
		t.Fatalf("media redirect and PNG cache entries = %d, err = %v", len(files), err)
	}
}

func TestCDNLogoScope(t *testing.T) {
	base := "https://djeqr6to3dedg.cloudfront.net/repo-logos/sbx/trivy-kit/live/logo-1790074145819"
	for _, ext := range []string{".svg", ".png", ".jpg", ".jpeg", ".webp"} {
		u, err := url.Parse(base + ext)
		if err != nil || !cacheableURL(u) {
			t.Fatalf("supported logo %s rejected: %v", ext, err)
		}
	}
	for _, target := range []string{base + ".gif", base + ".png?Signature=secret", "https://other.cloudfront.net/repo-logos/sbx/trivy-kit/live/logo-1.png", "https://djeqr6to3dedg.cloudfront.net/repo-logos/private/trivy-kit/live/logo-1.png"} {
		u, err := url.Parse(target)
		if err != nil {
			t.Fatal(err)
		}
		if cacheableURL(u) {
			t.Fatalf("unsafe logo URL accepted: %s", target)
		}
	}
}

func TestPublicHubCookieStrippedFromDisk(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return reply(http.StatusOK, `{"results":[]}`, http.Header{
			"Content-Type": {"application/json"}, "Set-Cookie": {"__cf_bm=secret"},
		}), nil
	})
	opts := Options{Dir: dir, Transport: base}
	client := NewClientWithOptions(time.Second, opts)
	read(t, client, catalogURL)
	resp, err := NewClientWithOptions(time.Second, opts).Get(catalogURL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if calls.Load() != 1 || resp.Header.Get("Set-Cookie") != "" {
		t.Fatalf("cached cookie response: calls = %d, cookie = %q", calls.Load(), resp.Header.Get("Set-Cookie"))
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("cache files = %v, err = %v", files, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "Set-Cookie") {
		t.Fatal("cookie persisted")
	}
}

func TestSignedBlobRedirectCachesUnderPublicURL(t *testing.T) {
	dir := t.TempDir()
	public := "https://registry-1.docker.io/v2/sbx/openclaw-kit/blobs/sha256:" + strings.Repeat("a", 64)
	signed := "https://production.cloudfront.docker.com/registry-v2/docker/registry/v2/blobs/sha256/a/data?Expires=123&Signature=secret&Key-Pair-Id=key"
	var calls atomic.Int32
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		switch req.URL.String() {
		case public:
			if req.Header.Get("Authorization") != "Bearer short-lived" {
				t.Error("registry auth header missing")
			}
			return reply(http.StatusTemporaryRedirect, "", http.Header{"Location": {signed}}), nil
		case signed:
			if req.Header.Get("Authorization") != "" {
				t.Error("registry auth leaked to CDN")
			}
			return reply(http.StatusOK, `{"kind":"sandbox"}`, http.Header{"Content-Type": {"application/octet-stream"}}), nil
		default:
			return nil, errors.New("unexpected URL: " + req.URL.String())
		}
	})
	opts := Options{Dir: dir, Transport: base}
	client := NewClientWithOptions(time.Second, opts)
	for range 2 {
		req, err := http.NewRequest(http.MethodGet, public, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer short-lived")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil || string(body) != `{"kind":"sandbox"}` {
			t.Fatalf("blob body = %q, err = %v", body, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("network calls = %d", calls.Load())
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("cache files = %v, err = %v", files, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), signed) || !strings.Contains(string(data), public) {
		t.Fatal("signed URL or credentials persisted, or public key missing")
	}
}

func TestAuthTokenAndUnsafeRequestsBypassCache(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return reply(http.StatusOK, `{"token":"secret"}`, nil), nil
	})
	client := NewClientWithOptions(time.Second, Options{Dir: dir, Transport: base})
	url := "https://auth.docker.io/token?service=registry.docker.io&scope=repository%3Asbx%2Fopenclaw-kit%3Apull"
	for range 2 {
		read(t, client, url)
	}
	for range 2 {
		req, err := http.NewRequest(http.MethodGet, catalogURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer private")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	if calls.Load() != 4 {
		t.Fatalf("network calls = %d", calls.Load())
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("unsafe request wrote %d cache files", len(files))
	}
}

func TestFailedRefreshPreservesFile(t *testing.T) {
	dir := t.TempDir()
	var fail atomic.Bool
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		if fail.Load() {
			return nil, errors.New("offline")
		}
		return reply(http.StatusOK, "cached", nil), nil
	})
	opts := Options{Dir: dir, Transport: base}
	if got := read(t, NewClientWithOptions(time.Second, opts), catalogURL); got != "cached" {
		t.Fatal(got)
	}
	fail.Store(true)
	if _, err := NewRefreshClientWithOptions(time.Second, opts).Get(catalogURL); err == nil {
		t.Fatal("refresh should fail")
	}
	if got := read(t, NewClientWithOptions(time.Second, opts), catalogURL); got != "cached" {
		t.Fatalf("old entry lost: %q", got)
	}
}

func TestBoundedBodyAndErrorResponsesNotStored(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) <= 2 {
			return reply(http.StatusOK, "oversized", nil), nil
		}
		return reply(http.StatusUnauthorized, "challenge", http.Header{"Www-Authenticate": {"Bearer"}}), nil
	})
	client := NewClientWithOptions(time.Second, Options{Dir: dir, Transport: base, MaxBodyBytes: 4})
	for range 2 {
		if got := read(t, client, catalogURL); got != "oversized" {
			t.Fatal(got)
		}
	}
	for range 2 {
		resp, err := client.Get(catalogURL)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatal(resp.StatusCode)
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("network calls = %d", calls.Load())
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatalf("stored oversized or error response")
	}
}

func TestCanceledWaiterAndConcurrentRequests(t *testing.T) {
	dir := t.TempDir()
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		return reply(http.StatusOK, "shared", nil), nil
	})
	opts := Options{Dir: dir, Transport: base}
	client := NewClientWithOptions(time.Second, opts)
	firstDone := make(chan error, 1)
	go func() {
		resp, err := client.Get(catalogURL)
		if err == nil {
			_ = resp.Body.Close()
		}
		firstDone <- err
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(req); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request = %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := NewClientWithOptions(time.Second, opts).Get(catalogURL)
			if err != nil {
				results <- err
				return
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				results <- err
			} else if string(body) != "shared" {
				results <- errors.New("wrong cached body")
			}
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		t.Error(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("network calls = %d", calls.Load())
	}
}
