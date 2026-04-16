package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/gui"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/process"
)

//go:embed icon.png
var iconBytes []byte

func init() {
	// When launched as a GUI app (Spotlight, Finder, Dock), macOS gives a
	// minimal PATH (/usr/bin:/bin:/usr/sbin:/sbin). Tools like sbx, gh,
	// glab, code, and go are typically in /usr/local/bin, /opt/homebrew/bin,
	// or ~/go/bin. Expand PATH so exec.LookPath and exec.Command find them.
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		home, _ := os.UserHomeDir()
		extra := []string{
			"/usr/local/bin",
			"/opt/homebrew/bin",
			"/opt/homebrew/sbin",
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "go", "bin"),
			filepath.Join(home, ".docker", "bin"),
		}
		current := os.Getenv("PATH")
		for _, dir := range extra {
			if !strings.Contains(current, dir) {
				current = current + ":" + dir
			}
		}
		_ = os.Setenv("PATH", current)
	}
}

func main() {
	var versionFlag bool
	var refreshFlag time.Duration
	flag.BoolVar(&versionFlag, "version", false, "Print version and exit")
	flag.BoolVar(&versionFlag, "v", false, "Print version and exit (shorthand)")
	flag.DurationVar(&refreshFlag, "refresh", 0, "Network refresh interval (e.g. 30s, 1m)")
	flag.DurationVar(&refreshFlag, "r", 0, "Network refresh interval (shorthand)")
	flag.Parse()

	if versionFlag {
		fmt.Println("biomelab", version)
		return
	}

	refreshInterval := resolveRefreshInterval(refreshFlag)

	detector := agent.NewDetector()
	ideDetector := ide.NewDetector()
	procLister := &process.OSLister{}
	configPath := config.DefaultPath()

	// If we're in a git repo, auto-add it to config.
	cwd, err := os.Getwd()
	if err == nil {
		repoRoot, repoErr := git.RepoRoot(cwd)
		if repoErr == nil {
			repo, openErr := git.OpenRepository(repoRoot)
			if openErr == nil {
				cfg, _ := config.Load(configPath)
				if cfg.Add(repoRoot, repo.RepoName(), config.ModeEntry{Type: "regular"}) {
					_ = config.Save(configPath, cfg)
				}
			}
		}
	}

	gui.AppIcon = &fyne.StaticResource{StaticName: "icon.png", StaticContent: iconBytes}

	app := gui.NewApp(configPath, detector, ideDetector, procLister, refreshInterval)
	app.Run()
}
