package git

import (
	"context"
	"os"
	"strings"
	"testing"
)

func isolateCredentialHelper(t *testing.T, script string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_COUNT", "2")
	// Empty helper clears previously configured helpers; only the test helper runs.
	t.Setenv("GIT_CONFIG_KEY_0", "credential.helper")
	t.Setenv("GIT_CONFIG_VALUE_0", "")
	t.Setenv("GIT_CONFIG_KEY_1", "credential.helper")
	t.Setenv("GIT_CONFIG_VALUE_1", "!f() { "+script+"; }; f")
}

func TestCredentialFillIsNoninteractive(t *testing.T) {
	// Git executes shell credential helpers on all supported platforms. Verify
	// the actual helper's environment and protocol rather than command flags.
	isolateCredentialHelper(t, `test "$GIT_TERMINAL_PROMPT" = 0 && test "$GCM_INTERACTIVE" = never || exit 1; input=$(cat); case "$input" in *host=example.invalid*) ;; *) exit 1;; esac; printf 'username=test-user\npassword=test-token\n'`)
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("GCM_INTERACTIVE", "always")
	auth, err := credentialFillContext(context.Background(), "https://example.invalid/org/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if auth.Username != "test-user" || auth.Password != "test-token" {
		t.Fatal("credential protocol was not preserved")
	}
}

func TestCredentialFailureDoesNotExposeOutput(t *testing.T) {
	isolateCredentialHelper(t, `printf 'password=private-test-value\n'; printf 'private-test-value' >&2; exit 1`)
	_, err := credentialFillContext(context.Background(), "https://example.invalid/org/repo.git")
	if err == nil || strings.Contains(err.Error(), "private-test-value") {
		t.Fatalf("unsafe or missing error: %v", err)
	}
}

func TestCredentialFillHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := credentialFillContext(ctx, "https://example.invalid/org/repo.git")
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
