package kits

import (
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
