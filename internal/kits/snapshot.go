package kits

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const snapshotVersion = 1
const snapshotTTL = time.Hour
const maxSnapshotSize = 4 << 20

// catalogSnapshot is replaced only after complete registry discovery succeeds.
// The HTTP cache can contain individually refreshed responses; this snapshot
// remains the last complete, usable view if a later refresh fails partway.
type catalogSnapshot struct {
	Version   int       `json:"version"`
	SavedAt   time.Time `json:"saved_at"`
	Sandboxes []Kit     `json:"sandboxes"`
	Mixins    []Kit     `json:"mixins"`
}

func snapshotPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".biomelab", "cache", "kits", "catalog-snapshot.json")
}

func fetchAvailableWithSnapshot(ctx context.Context, client *http.Client, hub, registry, auth, path string, now time.Time) ([]Kit, []Kit, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	saved, savedErr := readSnapshot(path)
	if savedErr == nil && !now.Before(saved.SavedAt) && now.Sub(saved.SavedAt) < snapshotTTL {
		return saved.Sandboxes, saved.Mixins, nil
	}
	sandboxes, mixins, err := fetchAvailableWithClient(ctx, client, hub, registry, auth)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if savedErr == nil {
			return saved.Sandboxes, saved.Mixins, nil
		}
		return nil, nil, err
	}
	// A non-writable cache must not hide a successfully discovered catalog.
	_ = writeSnapshot(path, sandboxes, mixins, time.Now())
	return sandboxes, mixins, nil
}

func readSnapshot(path string) (catalogSnapshot, error) {
	if path == "" {
		return catalogSnapshot{}, fmt.Errorf("catalog snapshot path unavailable")
	}
	f, err := os.Open(path)
	if err != nil {
		return catalogSnapshot{}, err
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, maxSnapshotSize+1))
	if err != nil {
		return catalogSnapshot{}, fmt.Errorf("read catalog snapshot: %w", err)
	}
	if len(body) > maxSnapshotSize {
		return catalogSnapshot{}, fmt.Errorf("catalog snapshot exceeds %d bytes", maxSnapshotSize)
	}
	var saved catalogSnapshot
	if err := json.Unmarshal(body, &saved); err != nil {
		return catalogSnapshot{}, fmt.Errorf("parse catalog snapshot: %w", err)
	}
	if saved.Version != snapshotVersion || saved.SavedAt.IsZero() {
		return catalogSnapshot{}, fmt.Errorf("unsupported or incomplete catalog snapshot")
	}
	for _, k := range saved.Sandboxes {
		if !validSnapshotKit(k, KindSandbox) {
			return catalogSnapshot{}, fmt.Errorf("invalid sandbox in catalog snapshot")
		}
	}
	for _, k := range saved.Mixins {
		if !validSnapshotKit(k, KindMixin) {
			return catalogSnapshot{}, fmt.Errorf("invalid mixin in catalog snapshot")
		}
	}
	return saved, nil
}

func validSnapshotKit(k Kit, kind string) bool {
	return k.Kind == kind && k.Name != "" &&
		strings.HasPrefix(k.Reference, "docker.io/sbx/") &&
		strings.HasSuffix(k.Reference, ":"+DefaultTag)
}

func writeSnapshot(path string, sandboxes, mixins []Kit, now time.Time) error {
	if path == "" {
		return fmt.Errorf("catalog snapshot path unavailable")
	}
	saved := catalogSnapshot{
		Version: snapshotVersion, SavedAt: now,
		Sandboxes: sandboxes, Mixins: mixins,
	}
	body, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("encode catalog snapshot: %w", err)
	}
	if len(body) > maxSnapshotSize {
		return fmt.Errorf("catalog snapshot exceeds %d bytes", maxSnapshotSize)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create catalog cache: %w", err)
	}
	f, err := os.CreateTemp(dir, ".catalog-snapshot-*.tmp")
	if err != nil {
		return fmt.Errorf("create catalog snapshot: %w", err)
	}
	tempName := f.Name()
	defer os.Remove(tempName)
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace catalog snapshot: %w", err)
	}
	return nil
}
