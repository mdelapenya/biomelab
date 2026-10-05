// Package resourcecache stores public Docker sbx kit HTTP resources on disk.
// Only the catalog, public sbx registry objects, and Docker Hub kit logos are
// eligible. In particular, authentication tokens and arbitrary HTTP responses
// never enter the cache.
package resourcecache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultTTL     = time.Hour
	defaultMaxBody = 8 << 20
	entryVersion   = 1
)

var (
	locks    sync.Map // absolute cache filename -> chan struct{}, one in-process writer per URL
	repoName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	cdnLogo  = regexp.MustCompile(`^/repo-logos/sbx/[a-z0-9][a-z0-9._-]*/live/logo-[A-Za-z0-9_-]+\.(svg|png|jpe?g|webp)$`)
)

// Options overrides defaults for tests or a caller with a custom cache root.
// Dir defaults to ~/.biomelab/cache/kits; TTL defaults to one hour. Transport
// defaults to http.DefaultTransport, and MaxBodyBytes defaults to 8 MiB.
type Options struct {
	Dir          string
	TTL          time.Duration
	Transport    http.RoundTripper
	Now          func() time.Time
	MaxBodyBytes int64
}

// NewClient returns a client with a persistent, one-hour public kit cache.
func NewClient(timeout time.Duration) *http.Client {
	return NewClientWithOptions(timeout, Options{})
}

// NewRefreshClient forces eligible resources to be fetched on the network and
// updates the cache on successful responses. Existing files remain on disk if
// refresh fails. It is intended for the app's startup kit preload.
func NewRefreshClient(timeout time.Duration) *http.Client {
	return newClient(timeout, Options{}, true)
}

// NewClientWithOptions permits an isolated directory, transport and clock in
// tests. It has the same cache behavior as NewClient.
func NewClientWithOptions(timeout time.Duration, options Options) *http.Client {
	return newClient(timeout, options, false)
}

// NewRefreshClientWithOptions permits isolated startup-refresh tests.
func NewRefreshClientWithOptions(timeout time.Duration, options Options) *http.Client {
	return newClient(timeout, options, true)
}

func newClient(timeout time.Duration, options Options, refresh bool) *http.Client {
	if options.Dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			options.Dir = filepath.Join(home, ".biomelab", "cache", "kits")
		}
	}
	if options.TTL <= 0 {
		options.TTL = defaultTTL
	}
	if options.Transport == nil {
		options.Transport = http.DefaultTransport
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.MaxBodyBytes <= 0 {
		options.MaxBodyBytes = defaultMaxBody
	}
	return &http.Client{
		Timeout: timeout, Transport: &transport{options: options, refresh: refresh},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if len(via) == 1 && registryBlobURL(via[0].URL) && signedBlobCDNURL(req.URL) {
				// Let net/http follow the redirect so Response.Request and
				// redirect behavior stay intact. Carry only the public origin
				// URL to the final transport call; never save the signed URL.
				*req = *req.WithContext(context.WithValue(req.Context(), blobOriginKey{}, via[0].URL.String()))
				req.Header.Del("Authorization")
			}
			return nil
		},
	}
}

type blobOriginKey struct{}

type transport struct {
	options Options
	refresh bool
}

type entry struct {
	Version int         `json:"version"`
	URL     string      `json:"url"`
	SavedAt time.Time   `json:"saved_at"`
	Status  int         `json:"status"`
	Header  http.Header `json:"header"`
	Body    []byte      `json:"body"`
	Digest  [32]byte    `json:"digest"`
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("resourcecache: nil request")
	}
	if req.Context().Err() != nil {
		return nil, req.Context().Err()
	}
	cacheURL := req.URL.String()
	redirectedBlob := false
	if source, ok := req.Context().Value(blobOriginKey{}).(string); ok && signedBlobCDNURL(req.URL) {
		if parsed, err := url.Parse(source); err == nil && registryBlobURL(parsed) {
			cacheURL = source
			redirectedBlob = true
		}
	}
	if t.options.Dir == "" || (!redirectedBlob && !cacheableRequest(req)) {
		return t.options.Transport.RoundTrip(req)
	}
	key := sha256.Sum256([]byte(cacheURL))
	filename := filepath.Join(t.options.Dir, fmt.Sprintf("%x.json", key))
	unlock, err := acquire(req.Context(), filename)
	if err != nil {
		return nil, err
	}
	defer unlock()

	if !t.refresh && !redirectedBlob {
		if saved, err := readEntry(filename, cacheURL, t.options.MaxBodyBytes); err == nil && fresh(saved, t.options.Now(), t.options.TTL) {
			return saved.response(req), nil
		}
	}

	resp, err := t.options.Transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if !cacheableResponse(req, resp) {
		return resp, nil
	}
	// Read at most the cache limit plus one byte. An oversized response is
	// returned intact to the caller without storing it.
	prefix, readErr := io.ReadAll(io.LimitReader(resp.Body, t.options.MaxBodyBytes+1))
	if readErr != nil {
		resp.Body = &combinedBody{Reader: io.MultiReader(bytes.NewReader(prefix), &onceError{err: readErr}), Closer: resp.Body}
		return resp, nil
	}
	if int64(len(prefix)) > t.options.MaxBodyBytes {
		resp.Body = &combinedBody{Reader: io.MultiReader(bytes.NewReader(prefix), resp.Body), Closer: resp.Body}
		return resp, nil
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(prefix))
	if !cacheableLogoBody(req.URL, prefix) {
		return resp, nil
	}
	saved := entry{
		Version: entryVersion, URL: cacheURL, SavedAt: t.options.Now(),
		Status: resp.StatusCode, Header: safeHeaders(resp.Header), Body: prefix,
		Digest: sha256.Sum256(prefix),
	}
	_ = writeEntry(filename, saved) // cache failures must not fail the request
	return resp, nil
}

type combinedBody struct {
	io.Reader
	io.Closer
}

type onceError struct{ err error }

func (r *onceError) Read([]byte) (int, error) {
	err := r.err
	r.err = io.EOF
	return 0, err
}

func acquire(ctx context.Context, key string) (func(), error) {
	v, _ := locks.LoadOrStore(key, make(chan struct{}, 1))
	lock := v.(chan struct{})
	select {
	case lock <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-lock
			return nil, err
		}
		return func() { <-lock }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func fresh(e entry, now time.Time, ttl time.Duration) bool {
	age := now.Sub(e.SavedAt)
	return age >= 0 && age < ttl
}

func readEntry(filename, requestURL string, maxBody int64) (entry, error) {
	var e entry
	f, err := os.Open(filename)
	if err != nil {
		return e, err
	}
	defer f.Close()
	// JSON adds base64 overhead; the cap also bounds malformed files.
	data, err := io.ReadAll(io.LimitReader(f, maxBody*2+4096))
	if err != nil || int64(len(data)) >= maxBody*2+4096 {
		return e, errors.New("resourcecache: invalid entry size")
	}
	if err := json.Unmarshal(data, &e); err != nil {
		return e, err
	}
	if e.Version != entryVersion || e.URL != requestURL || !allowedStatus(e.Status) || int64(len(e.Body)) > maxBody || sha256.Sum256(e.Body) != e.Digest || !safeStoredHeaders(e.Header) {
		return e, errors.New("resourcecache: invalid entry")
	}
	if e.Status == http.StatusMovedPermanently || e.Status == http.StatusFound {
		location, err := url.Parse(e.Header.Get("Location"))
		if err != nil || !location.IsAbs() || !cacheableURL(location) {
			return e, errors.New("resourcecache: invalid redirect")
		}
	}
	return e, nil
}

func (e entry) response(req *http.Request) *http.Response {
	return &http.Response{
		Status: fmt.Sprintf("%d %s", e.Status, http.StatusText(e.Status)), StatusCode: e.Status,
		Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: e.Header.Clone(), Body: io.NopCloser(bytes.NewReader(e.Body)),
		ContentLength: int64(len(e.Body)), Request: req,
	}
}

func writeEntry(filename string, e entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".kit-*")
	if err != nil {
		return err
	}
	tempName := f.Name()
	defer os.Remove(tempName)
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, filename)
}

func allowedStatus(status int) bool {
	return status == http.StatusOK || status == http.StatusMovedPermanently || status == http.StatusFound
}

func cacheableResponse(req *http.Request, resp *http.Response) bool {
	if resp == nil || resp.Body == nil || !allowedStatus(resp.StatusCode) {
		return false
	}
	// Cloudflare adds a bot-management cookie to public Hub catalog/media
	// responses. It does not affect their public content; never save it.
	if (resp.Header.Get("Set-Cookie") != "" && !publicHubURL(req.URL)) || resp.Header.Get("WWW-Authenticate") != "" || strings.Contains(strings.ToLower(resp.Header.Get("Cache-Control")), "private") || strings.Contains(strings.ToLower(resp.Header.Get("Cache-Control")), "no-store") {
		return false
	}
	if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusFound {
		location, err := url.Parse(resp.Header.Get("Location"))
		return err == nil && location.IsAbs() && location.Scheme == "https" && cacheableURL(location)
	}
	return true
}

var savedHeaderNames = []string{"Content-Type", "Content-Encoding", "Docker-Content-Digest", "ETag", "Last-Modified", "Location"}

func safeHeaders(h http.Header) http.Header {
	out := make(http.Header)
	for _, name := range savedHeaderNames {
		if values := h.Values(name); len(values) > 0 {
			out[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
		}
	}
	return out
}

func safeStoredHeaders(h http.Header) bool {
	if h == nil {
		return true
	}
	for key := range h {
		allowed := false
		for _, name := range savedHeaderNames {
			if http.CanonicalHeaderKey(key) == http.CanonicalHeaderKey(name) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return true
}

func cacheableRequest(req *http.Request) bool {
	if req.Method != http.MethodGet || req.Body != nil || req.URL == nil || req.URL.User != nil || req.Header.Get("Cookie") != "" || req.Header.Get("Proxy-Authorization") != "" {
		return false
	}
	// Registry sbx objects are public, although the registry requires a short-
	// lived bearer token to read them. Never persist that token or request headers.
	if req.Header.Get("Authorization") != "" && !registryURL(req.URL) {
		return false
	}
	return cacheableURL(req.URL)
}

func cacheableURL(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	switch strings.ToLower(u.Host) {
	case "hub.docker.com":
		if u.Path == "/v2/repositories/sbx/" {
			for name, values := range u.Query() {
				if (name != "page" && name != "page_size") || len(values) != 1 {
					return false
				}
			}
			return true
		}
		const prefix = "/api/media/repos_logo/v1/sbx/"
		return strings.HasPrefix(u.Path, prefix) && repoName.MatchString(strings.TrimPrefix(u.Path, prefix)) && u.Query().Get("type") == "logo" && len(u.Query()) == 1
	case "registry-1.docker.io":
		return registryURL(u)
	case "djeqr6to3dedg.cloudfront.net":
		return u.RawQuery == "" && cdnLogo.MatchString(u.Path)
	default:
		return false
	}
}

func publicHubURL(u *url.URL) bool {
	return u != nil && strings.ToLower(u.Host) == "hub.docker.com" && cacheableURL(u)
}

func cacheableLogoBody(u *url.URL, body []byte) bool {
	if u == nil || strings.ToLower(u.Host) != "djeqr6to3dedg.cloudfront.net" {
		return true
	}
	switch strings.ToLower(filepath.Ext(u.Path)) {
	case ".png":
		return bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n"))
	case ".jpg", ".jpeg":
		return bytes.HasPrefix(body, []byte{0xff, 0xd8, 0xff})
	case ".webp":
		return len(body) >= 12 && bytes.Equal(body[:4], []byte("RIFF")) && bytes.Equal(body[8:12], []byte("WEBP"))
	case ".svg":
		return bytes.Contains(bytes.ToLower(body[:min(len(body), 1024)]), []byte("<svg"))
	default:
		return false
	}
}

func registryURL(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || strings.ToLower(u.Host) != "registry-1.docker.io" || u.RawQuery != "" {
		return false
	}
	const prefix = "/v2/sbx/"
	if !strings.HasPrefix(u.Path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, prefix), "/")
	return len(parts) == 3 && repoName.MatchString(parts[0]) &&
		((parts[1] == "manifests" && parts[2] != "" && !strings.Contains(parts[2], "/")) ||
			(parts[1] == "blobs" && strings.HasPrefix(parts[2], "sha256:") && len(parts[2]) == len("sha256:")+64))
}

func registryBlobURL(u *url.URL) bool {
	if !registryURL(u) {
		return false
	}
	return strings.Contains(u.Path, "/blobs/")
}

func signedBlobCDNURL(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || strings.ToLower(u.Host) != "production.cloudfront.docker.com" || u.User != nil {
		return false
	}
	const prefix = "/registry-v2/docker/registry/v2/blobs/"
	q := u.Query()
	return strings.HasPrefix(u.Path, prefix) && strings.HasSuffix(u.Path, "/data") &&
		q.Has("Signature") && q.Has("Key-Pair-Id") && (q.Has("Expires") || q.Has("Policy"))
}
