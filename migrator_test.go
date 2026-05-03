package bucketfill_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/n1kola-petrovic/bucketfill"
	"github.com/n1kola-petrovic/bucketfill/provider/fs"
)

// newMigratorEnv returns a Migrator + temp bucket root + cleanup. Resets the
// global migration registry so tests don't bleed into each other.
func newMigratorEnv(t *testing.T) (*bucketfill.Migrator, string) {
	t.Helper()
	bucketfill.ResetMigrations()
	t.Cleanup(bucketfill.ResetMigrations)

	root := t.TempDir()
	storage := fs.New(root)
	return bucketfill.NewMigrator(bucketfill.NewClient(storage, "test")), filepath.Join(root, "test")
}

func TestMigratorUp_AppliesInOrderAndPersistsState(t *testing.T) {
	m, bucketRoot := newMigratorEnv(t)

	var calls []int
	for _, v := range []int{2, 1, 3} { // intentionally out of order
		v := v
		bucketfill.Register(&bucketfill.Migration{
			Version: v,
			Up: func(ctx context.Context, c *bucketfill.Client) error {
				calls = append(calls, v)
				return nil
			},
			Down: func(ctx context.Context, c *bucketfill.Client) error { return nil },
		})
	}

	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if got, want := calls, []int{1, 2, 3}; !equalInts(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}

	// State file written
	if _, err := os.Stat(filepath.Join(bucketRoot, "_bucketfill_state.json")); err != nil {
		t.Fatalf("expected state file: %v", err)
	}
}

func TestMigratorUp_ResumesFromState(t *testing.T) {
	m, _ := newMigratorEnv(t)

	for _, v := range []int{1, 2} {
		bucketfill.Register(&bucketfill.Migration{
			Version: v,
			Up:      func(ctx context.Context, c *bucketfill.Client) error { return nil },
			Down:    func(ctx context.Context, c *bucketfill.Client) error { return nil },
		})
	}
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("first Up: %v", err)
	}

	// Add v3, ensure only v3 runs second time
	v3Ran := false
	bucketfill.Register(&bucketfill.Migration{
		Version: 3,
		Up: func(ctx context.Context, c *bucketfill.Client) error {
			v3Ran = true
			return nil
		},
		Down: func(ctx context.Context, c *bucketfill.Client) error { return nil },
	})
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if !v3Ran {
		t.Fatal("v3 didn't run on second Up")
	}
}

func TestMigratorDown_OneStep(t *testing.T) {
	m, _ := newMigratorEnv(t)

	rolledBack := []int{}
	for _, v := range []int{1, 2, 3} {
		v := v
		bucketfill.Register(&bucketfill.Migration{
			Version: v,
			Up:      func(ctx context.Context, c *bucketfill.Client) error { return nil },
			Down: func(ctx context.Context, c *bucketfill.Client) error {
				rolledBack = append(rolledBack, v)
				return nil
			},
		})
	}
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := m.Down(context.Background()); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if got, want := rolledBack, []int{3}; !equalInts(got, want) {
		t.Fatalf("rolledBack = %v, want %v", got, want)
	}
}

func TestMigratorDownTo_Multi(t *testing.T) {
	m, _ := newMigratorEnv(t)

	rolledBack := []int{}
	for _, v := range []int{1, 2, 3} {
		v := v
		bucketfill.Register(&bucketfill.Migration{
			Version: v,
			Up:      func(ctx context.Context, c *bucketfill.Client) error { return nil },
			Down: func(ctx context.Context, c *bucketfill.Client) error {
				rolledBack = append(rolledBack, v)
				return nil
			},
		})
	}
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := m.DownTo(context.Background(), 0); err != nil {
		t.Fatalf("DownTo(0): %v", err)
	}
	if got, want := rolledBack, []int{3, 2, 1}; !equalInts(got, want) {
		t.Fatalf("rolledBack = %v, want %v", got, want)
	}
}

func TestMigratorUp_FailureLeavesPartialState(t *testing.T) {
	m, _ := newMigratorEnv(t)

	wantErr := errors.New("boom")
	bucketfill.Register(&bucketfill.Migration{
		Version: 1,
		Up:      func(ctx context.Context, c *bucketfill.Client) error { return nil },
		Down:    func(ctx context.Context, c *bucketfill.Client) error { return nil },
	})
	bucketfill.Register(&bucketfill.Migration{
		Version: 2,
		Up:      func(ctx context.Context, c *bucketfill.Client) error { return wantErr },
		Down:    func(ctx context.Context, c *bucketfill.Client) error { return nil },
	})

	err := m.Up(context.Background())
	if err == nil {
		t.Fatal("expected error from v2")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error chain doesn't wrap %q: %v", wantErr, err)
	}

	// State should reflect v1 applied (write-after-each-success), not v2.
	bucketfill.ResetMigrations()
	bucketfill.Register(&bucketfill.Migration{
		Version: 1,
		Up: func(ctx context.Context, c *bucketfill.Client) error {
			t.Fatal("v1 should not re-run")
			return nil
		},
		Down: func(ctx context.Context, c *bucketfill.Client) error { return nil },
	})
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("second Up: %v", err)
	}
}

func TestMigratorUp_GapErrors(t *testing.T) {
	m, _ := newMigratorEnv(t)

	// Register v1 and v3 — no v2.
	bucketfill.Register(&bucketfill.Migration{
		Version: 1,
		Up:      func(ctx context.Context, c *bucketfill.Client) error { return nil },
		Down:    func(ctx context.Context, c *bucketfill.Client) error { return nil },
	})
	bucketfill.Register(&bucketfill.Migration{
		Version: 3,
		Up:      func(ctx context.Context, c *bucketfill.Client) error { return nil },
		Down:    func(ctx context.Context, c *bucketfill.Client) error { return nil },
	})

	err := m.Up(context.Background())
	if err == nil {
		t.Fatal("expected gap error")
	}
	if !strings.Contains(err.Error(), "gap") {
		t.Fatalf("error didn't mention gap: %v", err)
	}
}

func TestMigrator_WithLogger(t *testing.T) {
	m, _ := newMigratorEnv(t)
	bucketfill.Register(&bucketfill.Migration{
		Version: 1,
		Up:      func(ctx context.Context, c *bucketfill.Client) error { return nil },
		Down:    func(ctx context.Context, c *bucketfill.Client) error { return nil },
	})

	captured := &captureLogger{}
	mWithLog := m.WithLogger(captured)
	if err := mWithLog.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(captured.lines) == 0 {
		t.Fatal("expected captured log lines")
	}
	joined := strings.Join(captured.lines, "\n")
	if !strings.Contains(joined, "applying v1") {
		t.Fatalf("expected 'applying v1' in captured logs, got: %s", joined)
	}
}

type captureLogger struct {
	lines []string
}

func (c *captureLogger) Logf(format string, args ...any) {
	c.lines = append(c.lines, fmt.Sprintf(format, args...))
}

func TestRegisterFS_AutoRegistersDefaults(t *testing.T) {
	bucketfill.ResetMigrations()
	defer bucketfill.ResetMigrations()

	fsys := fstest.MapFS{
		"v1/data/seeds/a.txt": {Data: []byte("a")},
		"v2/data/seeds/b.txt": {Data: []byte("b")},
		"README.md":           {Data: []byte("ignored")},
		"v3/data/.keep":       {Data: nil}, // empty version
	}

	if err := bucketfill.RegisterFS(fsys); err != nil {
		t.Fatalf("RegisterFS: %v", err)
	}

	got := bucketfill.Migrations()
	if len(got) != 3 {
		t.Fatalf("got %d migrations, want 3", len(got))
	}
	for i, want := range []int{1, 2, 3} {
		if got[i].Version != want {
			t.Errorf("migrations[%d].Version = %d, want %d", i, got[i].Version, want)
		}
		if got[i].Up == nil || got[i].Down == nil {
			t.Errorf("v%d missing Up/Down", got[i].Version)
		}
		if got[i].Data == nil {
			t.Errorf("v%d Data is nil", got[i].Version)
		}
	}
}

func TestRegisterFS_PreservesExistingRegistrations(t *testing.T) {
	bucketfill.ResetMigrations()
	defer bucketfill.ResetMigrations()

	customRan := false
	bucketfill.Register(&bucketfill.Migration{
		Version: 2,
		Up: func(ctx context.Context, c *bucketfill.Client) error {
			customRan = true
			return nil
		},
		Down: func(ctx context.Context, c *bucketfill.Client) error { return nil },
	})

	fsys := fstest.MapFS{
		"v1/data/a.txt": {Data: []byte("a")},
		"v2/data/b.txt": {Data: []byte("b")}, // would default-register but v2 already exists
	}
	if err := bucketfill.RegisterFS(fsys); err != nil {
		t.Fatalf("RegisterFS: %v", err)
	}

	all := bucketfill.Migrations()
	if len(all) != 2 {
		t.Fatalf("got %d migrations, want 2", len(all))
	}

	// v2 should still be the user's custom one, not the default.
	root := t.TempDir()
	storage := fs.New(root)
	m := bucketfill.NewMigrator(bucketfill.NewClient(storage, "test"))
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !customRan {
		t.Error("custom v2.Up did not run — RegisterFS overwrote the explicit registration")
	}
}

func TestRegister_DuplicateVersionPanics(t *testing.T) {
	bucketfill.ResetMigrations()
	defer bucketfill.ResetMigrations()

	bucketfill.Register(&bucketfill.Migration{Version: 1})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate version")
		}
	}()
	bucketfill.Register(&bucketfill.Migration{Version: 1})
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
