package ops

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/mdelapenya/biomelab/internal/sandbox"
)

func TestEnsureSandbox(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake sbx executable uses a POSIX shell")
	}
	for _, tc := range []struct {
		name, listing, storedName, wantName       string
		kits                                      []string
		baseRef                                   string
		generated, wrapper                        bool
		listFail, createFail, wantCreate, wantErr bool
	}{
		{name: "plain", listing: `{"sandboxes":[]}`, wantName: "owner-repo-claude", wantCreate: true},
		{name: "generated name when not stored", listing: `{"sandboxes":[]}`, generated: true, wantCreate: true},
		{name: "kits", listing: `{"sandboxes":[]}`, wantName: "owner-repo-claude", wantCreate: true,
			kits: []string{"docker.io/sbx/code-server-kit:latest", "docker.io/sbx/playwright-kit:latest"}},
		{name: "legacy wrapper with mixin", listing: `{"sandboxes":[]}`, wantName: "owner-repo-claude", wantCreate: true, wrapper: true,
			kits: []string{"docker.io/sbx/playwright-kit:latest"}},
		{name: "sandbox kit base with mixin", listing: `{"sandboxes":[]}`, wantName: "owner-repo-claude", wantCreate: true,
			baseRef: "docker.io/sbx/claude-kit:latest", kits: []string{"docker.io/sbx/playwright-kit:latest"}},
		{name: "do not adopt alternate name", listing: `{"sandboxes":[{"name":"claude-owner-repo","status":"stopped"}]}`, wantName: "owner-repo-claude", wantCreate: true},
		{name: "reuse explicit legacy name", listing: `{"sandboxes":[{"name":"claude-owner-repo","status":"stopped"}]}`, storedName: "claude-owner-repo", wantName: "claude-owner-repo"},
		{name: "never replace existing for kits", listing: `{"sandboxes":[{"name":"owner-repo-claude","status":"running"}]}`,
			kits: []string{"docker.io/sbx/code-server-kit:latest"}, wantErr: true},
		{name: "invalid discovery", listing: `broken-json`, wantErr: true},
		{name: "daemon unavailable", listFail: true, wantErr: true},
		{name: "creation fails", listing: `{"sandboxes":[]}`, createFail: true, wantCreate: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "create-args")
			script := `#!/bin/sh
case "$1" in
  ls)
    if [ "$TEST_LIST_FAIL" = true ]; then echo 'daemon unavailable' >&2; exit 1; fi
    printf '%s\n' "$TEST_LISTING"
    ;;
  create)
    printf '%s\n' "$@" >> "$TEST_CREATE_LOG"
    if [ "$TEST_CREATE_FAIL" = true ]; then echo 'create failed' >&2; exit 1; fi
    ;;
  *) echo 'unexpected sbx operation' >&2; exit 1 ;;
esac
`
			if err := os.WriteFile(filepath.Join(dir, "sbx"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("TEST_LISTING", tc.listing)
			t.Setenv("TEST_CREATE_LOG", log)
			t.Setenv("TEST_LIST_FAIL", boolString(tc.listFail))
			t.Setenv("TEST_CREATE_FAIL", boolString(tc.createFail))
			stored := tc.storedName
			if stored == "" && !tc.generated {
				stored = "owner-repo-claude"
			}
			var name string
			var created bool
			var err error
			if tc.wrapper {
				name, created, err = EnsureSandbox("owner/repo", "/workspace/my repo", stored, "claude", tc.kits)
			} else {
				name, created, err = EnsureSandboxWithKit("owner/repo", "/workspace/my repo", stored, "claude", tc.baseRef, tc.kits)
			}
			wantName := tc.wantName
			if tc.generated {
				wantName = sandbox.GeneratedName("/workspace/my repo", "claude")
			} else if wantName == "" {
				wantName = stored
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && (name != wantName || created != tc.wantCreate) {
				t.Fatalf("name=%q created=%v", name, created)
			}
			data, readErr := os.ReadFile(log)
			if !tc.wantCreate {
				if !os.IsNotExist(readErr) {
					t.Fatalf("unexpected create: %s (%v)", data, readErr)
				}
				return
			}
			if readErr != nil {
				t.Fatal(readErr)
			}
			want := []string{"create", "--name", wantName}
			for _, ref := range tc.kits {
				want = append(want, "--kit", ref)
			}
			workload := "claude"
			if tc.baseRef != "" {
				workload = tc.baseRef
			}
			want = append(want, workload, "/workspace/my repo")
			if got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"); !reflect.DeepEqual(got, want) {
				t.Fatalf("args = %q, want %q", got, want)
			}
		})
	}
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
