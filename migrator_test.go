package bucketfill_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

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
