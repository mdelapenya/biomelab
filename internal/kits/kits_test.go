package kits

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestParseSpec_Agent(t *testing.T) {
	raw := []byte(`schemaVersion: "1"
kind: agent
name: trivy
displayName: Trivy
description: "Vulnerability scanner"
`)
	k, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if k.Kind != KindAgent {
		t.Errorf("Kind = %q, want %q", k.Kind, KindAgent)
	}
	if k.Name != "trivy" || k.DisplayName != "Trivy" {
		t.Errorf("got %+v", k)
	}
	if k.Extends != "" {
		t.Errorf("agent should not have Extends, got %q", k.Extends)
	}
}

func TestParseSpec_Mixin(t *testing.T) {
	raw := []byte(`schemaVersion: "1"
kind: mixin
name: code-server
extends: claude
displayName: code-server (web VS Code) with Claude Code
description: Runs code-server on port 8080
`)
	k, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if k.Kind != KindMixin {
		t.Errorf("Kind = %q, want %q", k.Kind, KindMixin)
	}
	if k.Extends != "claude" {
		t.Errorf("Extends = %q, want claude", k.Extends)
	}
}

func TestKit_OCIReference(t *testing.T) {
	for _, k := range []Kit{
		{Name: "code-server"},
		{Name: "different-manifest-name", Directory: "code-server"},
	} {
		got := k.OCIReference()
		want := "docker.io/sbx/code-server-kit:latest"
		if got != want {
			t.Errorf("OCIReference = %q, want %q", got, want)
		}
	}
	if got := (Kit{Name: "wrong", Directory: "wrong-kit", Reference: "docker.io/sbx/actual-repository:latest"}).OCIReference(); got != "docker.io/sbx/actual-repository:latest" {
		t.Errorf("explicit OCIReference = %q", got)
	}
}

func TestFetchAvailable_PagesKindsMetadataAndLogo(t *testing.T) {
	specs := map[string]string{
		"kiro-kit":        "schemaVersion: \"2\"\nkind: sandbox\nname: kiro\ndisplayName: Kiro\ndescription: Kiro sandbox\n",
		"code-server-kit": "schemaVersion: \"2\"\nkind: mixin\nname: code-server\ndisplayName: Code Server\nrequires:\n  agent: claude\n",
		"future-kit":      "schemaVersion: \"2\"\nkind: future\nname: future\n",
	}
	server := kitFixtureServer(t, specs, nil)
	defer server.Close()
	sandboxes, mixins, err := fetchAvailableWithClient(context.Background(), server.Client(), server.URL, server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(sandboxes) != 1 || len(mixins) != 1 {
		t.Fatalf("sandboxes=%+v, mixins=%+v", sandboxes, mixins)
	}
	if k := sandboxes[0]; k.Kind != KindSandbox || k.Name != "kiro" || k.DisplayName != "Kiro" || k.Description != "Kiro sandbox" || k.OCIReference() != "docker.io/sbx/kiro-kit:latest" || k.LogoURL != "https://cdn.example.test/kiro-kit.svg" {
		t.Errorf("sandbox = %+v", k)
	}
	if k := mixins[0]; k.Kind != KindMixin || k.Requires.Agent != "claude" || k.DisplayName != "Code Server" || k.Description != "Manifest fallback" || k.OCIReference() != "docker.io/sbx/code-server-kit:latest" {
		t.Errorf("mixin = %+v", k)
	}
}

// kitFixtureServer serves two Hub pages and registry artifacts. The failure
// hook can replace a response before the default fixture handles it.
func kitFixtureServer(t *testing.T, specs map[string]string, fail func(http.ResponseWriter, *http.Request) bool) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail != nil && fail(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v2/repositories/sbx/" && r.URL.Query().Get("page") == "2":
			fmt.Fprint(w, `{"next":null,"results":[{"name":"code-server-kit","content_types":["sbx_kit"],"description":"Hub fallback"},{"name":"future-kit","content_types":["sbx_kit"]}]}`)
		case r.URL.Path == "/v2/repositories/sbx/":
			json.NewEncoder(w).Encode(map[string]any{"next": server.URL + "/v2/repositories/sbx/?page=2", "results": []map[string]any{{"name": "kiro-image", "content_types": []string{"image"}}, {"name": "kiro-kit", "content_types": []string{"sbx_kit"}}}})
		case r.URL.Path == "/token":
			if strings.Contains(r.URL.Query().Get("scope"), "kiro-image") {
				t.Error("ordinary image requested registry token")
			}
			fmt.Fprint(w, `{"token":"fixture-token"}`)
		case strings.HasPrefix(r.URL.Path, "/v2/sbx/"):
			if r.Header.Get("Authorization") != "Bearer fixture-token" {
				t.Errorf("missing registry bearer for %s", r.URL.Path)
			}
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) != 6 {
				http.Error(w, "bad path", http.StatusBadRequest)
				return
			}
			name, action := parts[3], parts[4]
			spec, ok := specs[name]
			if !ok {
				t.Errorf("unexpected repository %s", name)
				http.NotFound(w, r)
				return
			}
			hash := sha256.Sum256([]byte(spec))
			digest := "sha256:" + hex.EncodeToString(hash[:])
			if action == "manifests" {
				json.NewEncoder(w).Encode(map[string]any{"artifactType": artifactType, "config": map[string]any{"mediaType": configType, "digest": digest}, "annotations": map[string]string{"org.opencontainers.image.description": "Manifest fallback"}})
			} else if action == "blobs" && parts[5] == digest {
				fmt.Fprint(w, spec)
			} else {
				http.NotFound(w, r)
			}
		case strings.HasPrefix(r.URL.Path, "/api/media/repos_logo/v1/"):
			name := strings.TrimPrefix(r.URL.Path, "/api/media/repos_logo/v1/sbx/")
			w.Header().Set("Location", "https://cdn.example.test/"+name+".svg")
			w.WriteHeader(http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	return server
}

func TestFetchAvailable_FailsClosed(t *testing.T) {
	specs := map[string]string{
		"kiro-kit":        "kind: sandbox\nname: kiro\n",
		"code-server-kit": "kind: mixin\nname: code-server\n",
		"future-kit":      "kind: future\nname: future\n",
	}
	for _, tc := range []struct {
		name, path string
		status     int
	}{
		{"page", "/v2/repositories/sbx/", http.StatusServiceUnavailable},
		{"auth", "/token", http.StatusUnauthorized},
		{"manifest", "/v2/sbx/kiro-kit/manifests/latest", http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := kitFixtureServer(t, specs, func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == tc.path {
					http.Error(w, "fixture failure", tc.status)
					return true
				}
				return false
			})
			defer server.Close()
			a, m, err := fetchAvailableWithClient(context.Background(), server.Client(), server.URL, server.URL, server.URL)
			if err == nil || len(a) != 0 || len(m) != 0 {
				t.Fatalf("sandboxes=%v mixins=%v err=%v", a, m, err)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", tc.status)) {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestFetchAvailable_Cancellation(t *testing.T) {
	started := make(chan struct{})
	server := kitFixtureServer(t, nil, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.Contains(r.URL.Path, "/manifests/") {
			select {
			case <-started:
			default:
				close(started)
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
		_, _, err := fetchAvailableWithClient(ctx, server.Client(), server.URL, server.URL, server.URL)
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestFetchAvailable_LogoUnavailable(t *testing.T) {
	specs := map[string]string{
		"kiro-kit":        "kind: sandbox\nname: kiro\n",
		"code-server-kit": "kind: mixin\nname: code-server\n",
		"future-kit":      "kind: future\nname: future\n",
	}
	server := kitFixtureServer(t, specs, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasPrefix(r.URL.Path, "/api/media/repos_logo/") {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return true
		}
		return false
	})
	defer server.Close()
	sandboxes, mixins, err := fetchAvailableWithClient(context.Background(), server.Client(), server.URL, server.URL, server.URL)
	if err != nil || len(sandboxes) != 1 || len(mixins) != 1 {
		t.Fatalf("sandboxes=%v mixins=%v err=%v", sandboxes, mixins, err)
	}
	if sandboxes[0].LogoURL != "" || mixins[0].LogoURL != "" {
		t.Fatalf("unavailable logos should be empty: %+v %+v", sandboxes[0], mixins[0])
	}
}

func TestFetchAvailable_LiveCatalog(t *testing.T) {
	if os.Getenv("BIOMELAB_LIVE_KITS") != "1" {
		t.Skip("set BIOMELAB_LIVE_KITS=1 to query Docker Hub")
	}
	sandboxes, mixins, err := FetchAvailable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sandboxes) == 0 || len(mixins) == 0 {
		t.Fatalf("empty kind in live catalog: %d sandboxes, %d mixins", len(sandboxes), len(mixins))
	}
	logos := 0
	formats := map[string]int{}
	for _, k := range append(sandboxes, mixins...) {
		if k.Reference == "" {
			t.Errorf("kit %q has no exact reference", k.Name)
		}
		if k.LogoURL != "" {
			logos++
			if dot := strings.LastIndex(k.LogoURL, "."); dot >= 0 {
				formats[k.LogoURL[dot:]]++
			}
		}
		if k.Name == "openclaw" {
			t.Logf("OpenClaw: ref=%s logo=%s", k.Reference, k.LogoURL)
		}
	}
	t.Logf("discovered %d sandbox kits and %d mixins, %d logos: %v", len(sandboxes), len(mixins), logos, formats)
}

func TestRefreshCache_Live(t *testing.T) {
	if os.Getenv("BIOMELAB_LIVE_KITS") != "1" {
		t.Skip("set BIOMELAB_LIVE_KITS=1 to refresh the local Docker Hub cache")
	}
	if err := RefreshCache(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFilterMixinsForAgent_Requires(t *testing.T) {
	k, err := ParseSpec([]byte("kind: mixin\nname: code-server\nrequires:\n  agent: claude\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := FilterMixinsForAgent([]Kit{k}, "codex"); len(got) != 0 {
		t.Fatal("Claude-only kit offered to Codex")
	}
	if got := FilterMixinsForAgent([]Kit{k}, "claude"); len(got) != 1 {
		t.Fatal("Claude-only kit missing for Claude")
	}
}

func TestFilterMixinsForAgent(t *testing.T) {
	mixins := []Kit{
		{Name: "claude-only", Kind: KindMixin, Extends: "claude"},
		{Name: "gemini-only", Kind: KindMixin, Extends: "gemini"},
		{Name: "universal", Kind: KindMixin, Extends: ""},
	}

	tests := []struct {
		agent string
		want  []string
	}{
		{"claude", []string{"claude-only", "universal"}},
		{"gemini", []string{"gemini-only", "universal"}},
		{"shell", []string{"universal"}},
	}
	for _, tc := range tests {
		t.Run(tc.agent, func(t *testing.T) {
			got := FilterMixinsForAgent(mixins, tc.agent)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tc.want), got)
			}
			for i, k := range got {
				if k.Name != tc.want[i] {
					t.Errorf("[%d] got %q, want %q", i, k.Name, tc.want[i])
				}
			}
		})
	}
}

func TestSortByDisplay(t *testing.T) {
	ks := []Kit{
		{Name: "z-name", DisplayName: "Apple"},
		{Name: "a-name", DisplayName: "Zebra"},
		{Name: "m-name"}, // no DisplayName → falls back to Name
	}
	sortByDisplay(ks)
	want := []string{"Apple", "m-name", "Zebra"}
	for i, w := range want {
		got := ks[i].DisplayName
		if got == "" {
			got = ks[i].Name
		}
		if got != w {
			t.Errorf("[%d] = %q, want %q", i, got, w)
		}
	}
}
