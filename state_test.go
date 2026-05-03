package bucketfill_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/n1kola-petrovic/bucketfill"
	"github.com/n1kola-petrovic/bucketfill/provider/fs"
)

func TestState_MissingTreatedAsZero(t *testing.T) {
	bucketfill.ResetMigrations()
	defer bucketfill.ResetMigrations()

	m := bucketfill.NewMigrator(bucketfill.NewClient(fs.New(t.TempDir()), "test"))
	if err := m.Status(context.Background()); err != nil {
		t.Fatalf("Status on empty bucket: %v", err)
	}
}

func TestState_PersistsAcrossRuns(t *testing.T) {
	bucketfill.ResetMigrations()
	defer bucketfill.ResetMigrations()

	root := t.TempDir()
	storage := fs.New(root)
	m := bucketfill.NewMigrator(bucketfill.NewClient(storage, "test"))

	bucketfill.Register(&bucketfill.Migration{
		Version: 1,
		Up:      func(ctx context.Context, c *bucketfill.Client) error { return nil },
		Down:    func(ctx context.Context, c *bucketfill.Client) error { return nil },
	})
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Read the state file directly
	data, err := os.ReadFile(filepath.Join(root, "test", "_bucketfill_state.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var s bucketfill.State
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.Version != 1 {
		t.Fatalf("state.Version = %d, want 1", s.Version)
	}
	if s.AppliedAt.IsZero() {
		t.Error("AppliedAt is zero")
	}
}
