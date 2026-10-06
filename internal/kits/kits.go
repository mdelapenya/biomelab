// Package kits discovers Docker Sandbox kits published in Docker Hub's sbx namespace.
package kits

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/mdelapenya/biomelab/internal/resourcecache"
)

const DefaultTag = "latest"

const (
	KindAgent    = "agent" // Legacy v1 specs only.
	KindSandbox  = "sandbox"
	KindMixin    = "mixin"
	artifactType = "application/vnd.docker.sandbox.kit.v2"
	configType   = "application/vnd.docker.sandbox.kit.v2.spec+yaml"
	manifestType = "application/vnd.oci.image.manifest.v1+json"
	maxBody      = 4 << 20
)

type Kit struct {
	Name        string `yaml:"name"`
	Kind        string `yaml:"kind"`
	DisplayName string `yaml:"displayName"`
	Description string `yaml:"description"`
	Extends     string `yaml:"extends"`
	Requires    struct {
		Agent string `yaml:"agent"`
	} `yaml:"requires"`
	Directory string `yaml:"-"`
	Reference string `yaml:"-"`
	LogoURL   string `yaml:"-"`
}

func (k Kit) OCIReference() string {
	if k.Reference != "" {
		return k.Reference
	}
	name := k.Directory
	if name == "" {
		name = k.Name
	}
	return "docker.io/sbx/" + name + "-kit:" + DefaultTag
}

type repository struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	ContentTypes []string `json:"content_types"`
	Logo         string   `json:"logo"`
}
type page struct {
	Next    string       `json:"next"`
	Results []repository `json:"results"`
}
type manifest struct {
	ArtifactType string `json:"artifactType"`
	Config       struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
	} `json:"config"`
	Annotations map[string]string `json:"annotations"`
}

const (
	hubBase      = "https://hub.docker.com"
	registryBase = "https://registry-1.docker.io"
	authBase     = "https://auth.docker.io"
)

// FetchAvailable lists every published v2 kit, with exact kinds from its spec.
func FetchAvailable(ctx context.Context) ([]Kit, []Kit, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	return fetchAvailableWithSnapshot(ctx, resourcecache.NewClient(12*time.Second),
		hubBase, registryBase, authBase, snapshotPath(), time.Now())
}

// RefreshCache updates all public catalog metadata and logo assets in the persistent cache.
func RefreshCache(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	client := resourcecache.NewRefreshClient(12 * time.Second)
	return refreshCacheWithClient(ctx, client, hubBase, registryBase, authBase, snapshotPath())
}

func refreshCacheWithClient(ctx context.Context, client *http.Client, hub, registry, auth, path string) error {
	sandboxes, mixins, err := fetchAvailableWithClient(ctx, client, hub, registry, auth)
	if err != nil {
		return err
	}
	// Catalog metadata is complete at this point. Publish it before optional
	// image downloads so a slow logo CDN cannot leave first-run users without
	// a usable kit picker.
	if err := writeSnapshot(path, sandboxes, mixins, time.Now()); err != nil {
		return err
	}
	all := append(sandboxes, mixins...)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	for _, kit := range all {
		if kit.LogoURL == "" {
			continue
		}
		logoURL := kit.LogoURL
		g.Go(func() error {
			if _, err := getBytes(gctx, client, logoURL, "", "image/svg+xml,image/png,image/webp,image/*"); err != nil {
				// Logos are optional, so a stale or missing logo cannot prevent catalog refresh.
				return nil
			}
			return nil
		})
	}
	_ = g.Wait()
	if ctx.Err() == context.Canceled {
		return context.Canceled
	}
	return nil
}

// fetchAvailableWithClient allows fixtures to supply a local HTTP server.
func fetchAvailableWithClient(ctx context.Context, client *http.Client, hub, registry, auth string) ([]Kit, []Kit, error) {
	repos, err := listRepositories(ctx, client, hub)
	if err != nil {
		return nil, nil, err
	}
	candidates := make([]repository, 0, len(repos))
	for _, r := range repos {
		for _, typ := range r.ContentTypes {
			if typ == "sbx_kit" {
				candidates = append(candidates, r)
				break
			}
		}
	}
	results := make([]Kit, len(candidates))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	for i, r := range candidates {
		i, r := i, r
		g.Go(func() error {
			k, err := resolveKit(gctx, client, hub, registry, auth, r)
			if err != nil {
				return fmt.Errorf("resolve sbx/%s: %w", r.Name, err)
			}
			results[i] = k
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}
	var sandboxes, mixins []Kit
	for _, k := range results {
		switch k.Kind {
		case KindSandbox:
			sandboxes = append(sandboxes, k)
		case KindMixin:
			mixins = append(mixins, k)
		}
	}
	sortByDisplay(sandboxes)
	sortByDisplay(mixins)
	return sandboxes, mixins, nil
}

func listRepositories(ctx context.Context, client *http.Client, hub string) ([]repository, error) {
	base, err := url.Parse(hub)
	if err != nil {
		return nil, fmt.Errorf("invalid Hub URL: %w", err)
	}
	next := strings.TrimRight(hub, "/") + "/v2/repositories/sbx/?page_size=100"
	seen := map[string]bool{}
	var repos []repository
	for next != "" {
		if seen[next] {
			return nil, fmt.Errorf("catalog pagination loop at %s", next)
		}
		seen[next] = true
		body, err := getBytes(ctx, client, next, "", "")
		if err != nil {
			return nil, fmt.Errorf("list Docker Hub kits: %w", err)
		}
		var p page
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, fmt.Errorf("parse catalog page %s: %w", next, err)
		}
		if p.Results == nil {
			return nil, fmt.Errorf("catalog page %s has no results", next)
		}
		repos = append(repos, p.Results...)
		if p.Next == "" {
			break
		}
		u, err := url.Parse(p.Next)
		if err != nil {
			return nil, fmt.Errorf("invalid catalog next page: %w", err)
		}
		u = base.ResolveReference(u)
		if u.Scheme != base.Scheme || u.Host != base.Host || !strings.HasPrefix(u.Path, "/v2/repositories/sbx/") {
			return nil, fmt.Errorf("catalog next page points outside sbx namespace: %s", u)
		}
		next = u.String()
	}
	return repos, nil
}

func resolveKit(ctx context.Context, client *http.Client, hub, registry, auth string, r repository) (Kit, error) {
	if r.Name == "" || strings.Contains(r.Name, "/") {
		return Kit{}, fmt.Errorf("invalid repository name %q", r.Name)
	}
	q := url.Values{"service": {"registry.docker.io"}, "scope": {"repository:sbx/" + r.Name + ":pull"}}
	body, err := getBytes(ctx, client, strings.TrimRight(auth, "/")+"/token?"+q.Encode(), "", "")
	if err != nil {
		return Kit{}, fmt.Errorf("request registry token: %w", err)
	}
	var token struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return Kit{}, fmt.Errorf("parse registry token: %w", err)
	}
	bearer := token.Token
	if bearer == "" {
		bearer = token.AccessToken
	}
	if bearer == "" {
		return Kit{}, fmt.Errorf("empty registry token")
	}
	path := strings.TrimRight(registry, "/") + "/v2/sbx/" + url.PathEscape(r.Name)
	body, err = getBytes(ctx, client, path+"/manifests/"+DefaultTag, bearer, manifestType)
	if err != nil {
		return Kit{}, fmt.Errorf("fetch latest manifest: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return Kit{}, fmt.Errorf("parse latest manifest: %w", err)
	}
	if m.ArtifactType != artifactType || m.Config.MediaType != configType {
		return Kit{}, fmt.Errorf("latest manifest is not a v2 sandbox kit (artifactType %q, config mediaType %q)", m.ArtifactType, m.Config.MediaType)
	}
	digest := m.Config.Digest
	if len(digest) != len("sha256:")+64 || !strings.HasPrefix(digest, "sha256:") {
		return Kit{}, fmt.Errorf("invalid kit spec digest %q", digest)
	}
	body, err = getBytes(ctx, client, path+"/blobs/"+digest, bearer, configType)
	if err != nil {
		return Kit{}, fmt.Errorf("fetch kit spec blob: %w", err)
	}
	hash := sha256.Sum256(body)
	if hex.EncodeToString(hash[:]) != strings.TrimPrefix(digest, "sha256:") {
		return Kit{}, fmt.Errorf("kit spec blob digest does not match manifest")
	}
	k, err := ParseSpec(body)
	if err != nil {
		return Kit{}, fmt.Errorf("parse kit spec blob: %w", err)
	}
	if k.Kind == "" {
		return Kit{}, fmt.Errorf("kit spec blob has no kind")
	}
	if k.Name == "" {
		k.Name = m.Annotations["vnd.docker.sandbox.kit.name"]
	}
	if k.Name == "" {
		k.Name = r.Name
	}
	if k.DisplayName == "" {
		k.DisplayName = m.Annotations["org.opencontainers.image.title"]
	}
	if k.DisplayName == "" {
		k.DisplayName = k.Name
	}
	if k.Description == "" {
		k.Description = m.Annotations["org.opencontainers.image.description"]
	}
	if k.Description == "" {
		k.Description = r.Description
	}
	k.Directory = r.Name
	k.Reference = "docker.io/sbx/" + r.Name + ":" + DefaultTag
	k.LogoURL = r.Logo
	if k.LogoURL == "" && (k.Kind == KindSandbox || k.Kind == KindMixin) {
		// Hub's stable media URL redirects to the current published logo when
		// one exists. Resolving it here would make optional image latency part
		// of required catalog discovery.
		k.LogoURL = strings.TrimRight(hub, "/") + "/api/media/repos_logo/v1/" + url.PathEscape("sbx/"+r.Name) + "?type=logo"
	}
	if err := ctx.Err(); err != nil {
		return Kit{}, err
	}
	return k, nil
}

func getBytes(ctx context.Context, client *http.Client, target, bearer, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		brief, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", target, resp.StatusCode, strings.TrimSpace(string(brief)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", target, err)
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("response from %s exceeds %d bytes", target, maxBody)
	}
	return body, nil
}

func FilterMixinsForAgent(mixins []Kit, agent string) []Kit {
	out := make([]Kit, 0, len(mixins))
	for _, m := range mixins {
		if (m.Requires.Agent == "" || m.Requires.Agent == agent) &&
			(m.Extends == "" || m.Extends == agent) {
			out = append(out, m)
		}
	}
	return out
}

func ParseSpec(raw []byte) (Kit, error) {
	var k Kit
	if err := yaml.Unmarshal(raw, &k); err != nil {
		return Kit{}, err
	}
	return k, nil
}

func sortByDisplay(ks []Kit) {
	sort.SliceStable(ks, func(i, j int) bool {
		ai, aj := ks[i].DisplayName, ks[j].DisplayName
		if ai == "" {
			ai = ks[i].Name
		}
		if aj == "" {
			aj = ks[j].Name
		}
		ai, aj = strings.ToLower(ai), strings.ToLower(aj)
		if ai == aj {
			return ks[i].Reference < ks[j].Reference
		}
		return ai < aj
	})
}
