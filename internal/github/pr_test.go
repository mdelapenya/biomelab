package github

import (
	"os/exec"
	"testing"
)

func TestCheckGH_NotFound(t *testing.T) {
	// Temporarily shadow PATH so gh cannot be found.
	t.Setenv("PATH", "")

	got := CheckGH()
	if got != GHNotFound {
		t.Errorf("expected GHNotFound, got %v", got)
	}
}

func TestCheckGH_Available(t *testing.T) {
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not available in this environment")
	}

	// We cannot assert GHAvailable without a real authenticated session,
	// but we can assert that the result is a valid GHAvailability value.
	got := CheckGH()
	if got != GHAvailable && got != GHNotAuthenticated {
		t.Errorf("unexpected GHAvailability value: %v", got)
	}
}

func TestGHAvailability_Constants(t *testing.T) {
	// Ensure the constants have distinct values so they can be used as
	// discriminated results.
	if GHAvailable == GHNotFound || GHAvailable == GHNotAuthenticated || GHNotFound == GHNotAuthenticated {
		t.Error("GHAvailability constants must be distinct")
	}
}

func TestRollupStatus_Empty(t *testing.T) {
	got := rollupStatus(nil)
	if got != "" {
		t.Errorf("expected empty string for no checks, got %q", got)
	}
}

func TestRollupStatus_AllSuccess(t *testing.T) {
	checks := []struct {
		State      string `json:"state"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}{
		{Conclusion: "success", Status: "completed"},
		{Conclusion: "success", Status: "completed"},
	}
	got := rollupStatus(checks)
	if got != "success" {
		t.Errorf("expected success, got %q", got)
	}
}

func TestRollupStatus_AnyFailure(t *testing.T) {
	checks := []struct {
		State      string `json:"state"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}{
		{Conclusion: "success", Status: "completed"},
		{Conclusion: "failure", Status: "completed"},
	}
	got := rollupStatus(checks)
	if got != "failure" {
		t.Errorf("expected failure, got %q", got)
	}
}

func TestRollupStatus_PendingWhenInProgress(t *testing.T) {
	checks := []struct {
		State      string `json:"state"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}{
		{Conclusion: "success", Status: "completed"},
		{Status: "in_progress"},
	}
	got := rollupStatus(checks)
	if got != "pending" {
		t.Errorf("expected pending, got %q", got)
	}
}

func TestRollupStatus_FailureTakesPrecedenceOverPending(t *testing.T) {
	checks := []struct {
		State      string `json:"state"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}{
		{Conclusion: "failure"},
		{Status: "in_progress"},
	}
	got := rollupStatus(checks)
	if got != "failure" {
		t.Errorf("expected failure to take precedence over pending, got %q", got)
	}
}

func TestStatusIcon(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"success", "✓"},
		{"failure", "✗"},
		{"pending", "●"},
		{"", ""},
		{"unknown", ""},
	}
	for _, tc := range cases {
		got := StatusIcon(tc.status)
		if got != tc.want {
			t.Errorf("StatusIcon(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
