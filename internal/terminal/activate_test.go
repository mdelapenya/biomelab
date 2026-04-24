package terminal

import "testing"

func TestTitle(t *testing.T) {
	got := Title("feature-branch")
	want := "biomelab: feature-branch"
	if got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
}

func TestTitleEscape(t *testing.T) {
	got := titleEscape("feature-branch")
	want := "printf '\\033]0;biomelab: feature-branch\\007'; "
	if got != want {
		t.Errorf("titleEscape() = %q, want %q", got, want)
	}
}

func TestBuildShellCmdWithTitle(t *testing.T) {
	got, err := buildShellCmdWithTitle("/project", "", "my-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "printf '\\033]0;biomelab: my-branch\\007'; cd '/project'; exec $SHELL"
	if got != want {
		t.Errorf("buildShellCmdWithTitle() = %q, want %q", got, want)
	}
}

func TestBuildShellCmdWithTitle_EmptyIdentifier(t *testing.T) {
	got, err := buildShellCmdWithTitle("/project", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No title prefix when identifier is empty.
	want := "cd '/project'; exec $SHELL"
	if got != want {
		t.Errorf("buildShellCmdWithTitle() = %q, want %q", got, want)
	}
}

func TestBuildShellCmdWithTitle_WithCommand(t *testing.T) {
	got, err := buildShellCmdWithTitle("", "sbx run mybox", "my-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "printf '\\033]0;biomelab: my-branch\\007'; sbx run mybox; exec $SHELL"
	if got != want {
		t.Errorf("buildShellCmdWithTitle() = %q, want %q", got, want)
	}
}

func TestDarwinAppNames_Coverage(t *testing.T) {
	// Verify that the common macOS terminal kinds have app name mappings.
	for _, kind := range []Kind{TerminalApp, ITerm2, Alacritty, Kitty, WezTerm} {
		if _, ok := darwinAppNames[kind]; !ok {
			t.Errorf("darwinAppNames missing entry for %q", kind)
		}
	}
}

func TestDarwinAppNames_NoLinuxOnly(t *testing.T) {
	// Linux-only emulators should NOT have macOS app name mappings.
	for _, kind := range []Kind{GnomeTerminal, Konsole, Tilix, Xfce4Terminal} {
		if name, ok := darwinAppNames[kind]; ok {
			t.Errorf("darwinAppNames should not contain Linux-only %q (mapped to %q)", kind, name)
		}
	}
}

func TestTitle_EmptyIdentifier(t *testing.T) {
	got := Title("")
	want := "biomelab: "
	if got != want {
		t.Errorf("Title(\"\") = %q, want %q", got, want)
	}
}

func TestTitle_SpecialCharacters(t *testing.T) {
	got := Title("feature/my-branch")
	want := "biomelab: feature/my-branch"
	if got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
}

func TestBuildShellCmdWithTitle_NoArgs(t *testing.T) {
	_, err := buildShellCmdWithTitle("", "", "my-branch")
	if err == nil {
		t.Error("expected error when both dir and command are empty")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"has space", "'has space'"},
		{"it's", "'it'\\''s'"},
		{"", "''"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := shellQuote(tt.input); got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildShellCmd(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		command string
		want    string
		wantErr bool
	}{
		{"dir only", "/project", "", "cd '/project'; exec $SHELL", false},
		{"command only", "", "sbx run mybox", "sbx run mybox; exec $SHELL", false},
		{"command takes precedence", "/project", "sbx run mybox", "sbx run mybox; exec $SHELL", false},
		{"neither", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildShellCmd(tt.dir, tt.command)
			if (err != nil) != tt.wantErr {
				t.Fatalf("buildShellCmd() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("buildShellCmd() = %q, want %q", got, tt.want)
			}
		})
	}
}
