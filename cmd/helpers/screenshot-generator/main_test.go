package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Rendering changes the application's shared theme palette. Keep these tests
// serial, and put all output under t.TempDir so CI never changes website assets.
func TestRunGeneratesDashboardImages(t *testing.T) {
	for _, customOutput := range []bool{false, true} {
		name := "default output"
		if customOutput {
			name = "custom nested output"
		}
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			outDir := filepath.Join("website", "img")
			var args []string
			if customOutput {
				outDir = filepath.Join(t.TempDir(), "with spaces", "images")
				args = []string{"-output-dir", outDir}
			}
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("exit code %d: %s", code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", stderr.String())
			}
			dark := readDashboard(t, filepath.Join(outDir, "dashboard-dark.png"))
			light := readDashboard(t, filepath.Join(outDir, "dashboard-light.png"))
			if brightness(dark) >= brightness(light) {
				t.Fatal("dark dashboard must be darker than light dashboard")
			}
			lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
			if len(lines) != 2 || lines[0] != filepath.Join(outDir, "dashboard-dark.png") || lines[1] != filepath.Join(outDir, "dashboard-light.png") {
				t.Fatalf("unexpected output paths: %q", stdout.String())
			}
			if customOutput {
				if _, err := os.Stat("website"); !os.IsNotExist(err) {
					t.Fatalf("custom output should not create default directory: %v", err)
				}
			}
		})
	}
}

func TestRunOverwritesImagesAndPreservesOtherFiles(t *testing.T) {
	outDir := t.TempDir()
	for _, name := range []string{"dashboard-dark.png", "dashboard-light.png", "keep.txt"} {
		if err := os.WriteFile(filepath.Join(outDir, name), []byte("existing content"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-output-dir", outDir}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d: %s", code, stderr.String())
	}
	readDashboard(t, filepath.Join(outDir, "dashboard-dark.png"))
	readDashboard(t, filepath.Join(outDir, "dashboard-light.png"))
	data, err := os.ReadFile(filepath.Join(outDir, "keep.txt"))
	if err != nil || string(data) != "existing content" {
		t.Fatalf("unrelated file changed: %q, %v", data, err)
	}
}

func TestRunUsageDoesNotGenerateImages(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		code    int
		message string
	}{
		{"help", []string{"-help"}, 0, "-output-dir"},
		{"unknown flag", []string{"-unknown"}, 2, "flag provided but not defined"},
		{"missing value", []string{"-output-dir"}, 2, "flag needs an argument"},
		{"positional argument", []string{"unexpected"}, 2, "unexpected positional arguments"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			var stdout, stderr bytes.Buffer
			if code := run(tc.args, &stdout, &stderr); code != tc.code {
				t.Fatalf("exit code = %d, want %d; stderr: %s", code, tc.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.message) || stdout.Len() != 0 {
				t.Fatalf("unexpected output: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("usage request created files: %v, %v", entries, err)
			}
		})
	}
}

func TestRunReportsOutputFailures(t *testing.T) {
	for _, destinationBlocked := range []bool{false, true} {
		name := "output directory is a file"
		if destinationBlocked {
			name = "image destination is a directory"
		}
		t.Run(name, func(t *testing.T) {
			outDir := t.TempDir()
			message := "write "
			if destinationBlocked {
				if err := os.Mkdir(filepath.Join(outDir, "dashboard-dark.png"), 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				outDir = filepath.Join(outDir, "blocked")
				if err := os.WriteFile(outDir, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
				message = "create output directory"
			}
			var stdout, stderr bytes.Buffer
			if code := run([]string{"-output-dir", outDir}, &stdout, &stderr); code != 1 {
				t.Fatalf("exit code = %d, want 1; stderr: %s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), message) || stdout.Len() != 0 {
				t.Fatalf("unexpected output: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func readDashboard(t *testing.T, path string) image.Image {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("invalid PNG %s: %v", path, err)
	}
	if img.Bounds().Dx() != 1440 || img.Bounds().Dy() != 720 {
		t.Fatalf("unexpected image dimensions: %v", img.Bounds())
	}
	// Reject blank output without relying on platform-specific font rasterization.
	colors := make(map[color.RGBA]bool)
	for y := 0; y < 720; y += 8 {
		for x := 0; x < 1440; x += 8 {
			colors[color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)] = true
		}
	}
	if len(colors) < 16 {
		t.Fatalf("dashboard appears blank: only %d sampled colors", len(colors))
	}
	return img
}

func brightness(img image.Image) uint64 {
	var total uint64
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y += 8 {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x += 8 {
			r, g, b, _ := img.At(x, y).RGBA()
			total += uint64(r) + uint64(g) + uint64(b)
		}
	}
	return total
}

// A closed output stream must not turn a failed invocation into success.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("output stream closed")
}

func TestRunReportsStdoutFailure(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"-output-dir", t.TempDir()}, failingWriter{}, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "report generated image") || !strings.Contains(stderr.String(), "output stream closed") {
		t.Fatalf("missing stdout failure diagnostic: %q", stderr.String())
	}
}

func TestRunPreservesFailureStatusWhenStderrFails(t *testing.T) {
	if code := run([]string{"unexpected"}, io.Discard, failingWriter{}); code != 2 {
		t.Fatalf("invalid arguments exit code = %d, want 2", code)
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"-output-dir", blocked}, io.Discard, failingWriter{}); code != 1 {
		t.Fatalf("generation failure exit code = %d, want 1", code)
	}
}
