package bucketfill

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
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
