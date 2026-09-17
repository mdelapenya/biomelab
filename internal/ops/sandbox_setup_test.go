package ops

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestEnsureSandbox(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake sbx executable uses a POSIX shell")
	}
	for _, tc := range []struct {
		name, listing, wantName                   string
		kits                                      []string
		listFail, createFail, wantCreate, wantErr bool
	}{
		{name: "plain", listing: `{"sandboxes":[]}`, wantName: "owner-repo-claude", wantCreate: true},
		{name: "kits", listing: `{"sandboxes":[]}`, wantName: "owner-repo-claude", wantCreate: true,
			kits: []string{"docker.io/sbx/code-server-kit:latest", "docker.io/sbx/playwright-kit:latest"}},
		{name: "reuse alternate name", listing: `{"sandboxes":[{"name":"claude-owner-repo","status":"stopped"}]}`, wantName: "claude-owner-repo"},
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
			name, created, err := EnsureSandbox("owner/repo", "/workspace/my repo", "owner-repo-claude", "claude", tc.kits)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && (name != tc.wantName || created != tc.wantCreate) {
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
			want := []string{"create", "--name", "owner-repo-claude"}
			for _, ref := range tc.kits {
				want = append(want, "--kit", ref)
			}
			want = append(want, "claude", "/workspace/my repo")
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
