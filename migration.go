package bucketfill

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"sync"
)

// MigrationFunc is the body of a single migration step.
type MigrationFunc func(ctx context.Context, client *Client) error

// Migration is a versioned bundle of an Up and Down function plus an optional
// data filesystem (the embedded data/ subtree for that version).
type Migration struct {
	Version int
	Up      MigrationFunc
	Down    MigrationFunc
	Data    fs.FS // rooted at the migration's data/ tree; nil if absent
}

var (
	mu         sync.Mutex
	migrations []*Migration
)

// Register adds a migration to the global registry. Typically called from the
// scaffolded entry binary's main(), once per version. Panics on duplicate version.
func Register(m *Migration) {
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range migrations {
		if existing.Version == m.Version {
			panic(fmt.Sprintf("bucketfill: duplicate migration version %d", m.Version))
		}
	}
	migrations = append(migrations, m)
}

// Migrations returns a copy of all registered migrations in ascending version order.
func Migrations() []*Migration {
	mu.Lock()
	defer mu.Unlock()
	sorted := make([]*Migration, len(migrations))
	copy(sorted, migrations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Version < sorted[j].Version })
	return sorted
}

// ResetMigrations clears the registry. Tests use this between subtests.
func ResetMigrations() {
	mu.Lock()
	defer mu.Unlock()
	migrations = nil
}

// DefaultUp is the migration body used when no explicit Up is provided.
// It mirrors the per-version data/ tree into the bucket via Client.PutAll.
func DefaultUp(ctx context.Context, c *Client) error { return c.PutAll(ctx) }

// DefaultDown is the symmetric default rollback: it deletes from the bucket
// every key the per-version data/ tree contains, via Client.DeleteAll.
func DefaultDown(ctx context.Context, c *Client) error { return c.DeleteAll(ctx) }

var versionDirRE = regexp.MustCompile(`^v(\d+)$`)

// RegisterFS scans fsys for top-level v<N>/ directories and auto-registers a
// default migration for each one whose data/ subtree exists. The defaults are
// DefaultUp (PutAll) and DefaultDown (DeleteAll).
//
// Versions already in the registry (added via Register before this call) are
// left untouched — this is how callers override the defaults for specific
// versions that need custom Up/Down logic.
//
// Typical use, with the migrations package shipping a single embed.FS:
//
//	//go:embed all:v*
//	var FS embed.FS
//
//	// in cmd/migrate/main.go:
//	bucketfill.RegisterFS(migrations.FS)
func RegisterFS(fsys fs.FS) error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("bucketfill: read FS: %w", err)
	}

	mu.Lock()
	existing := make(map[int]bool, len(migrations))
	for _, m := range migrations {
		existing[m.Version] = true
	}
	mu.Unlock()

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		match := versionDirRE.FindStringSubmatch(e.Name())
		if match == nil {
			continue
		}
		v, _ := strconv.Atoi(match[1])
		if v <= 0 || existing[v] {
			continue
		}
		dataPath := e.Name() + "/data"
		if _, err := fs.Stat(fsys, dataPath); err != nil {
			// No data subdir for this version — skip; user likely registered
			// a custom Up/Down already, or the directory is incomplete.
			continue
		}
		dataFS, err := fs.Sub(fsys, dataPath)
		if err != nil {
			return fmt.Errorf("bucketfill: sub %s: %w", dataPath, err)
		}
		Register(&Migration{
			Version: v,
			Up:      DefaultUp,
			Down:    DefaultDown,
			Data:    dataFS,
		})
	}
	return nil
}
