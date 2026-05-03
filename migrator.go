package bucketfill

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Logger is a minimal interface bucketfill writes progress messages through.
// The default implementation prints to os.Stdout in the bucketfill format.
//
// To plug in your own logger, implement Logf and pass it via Migrator.WithLogger.
// A typical adapter for a structured logger:
//
//	type adapter struct{ l *mypkg.Logger }
//	func (a adapter) Logf(format string, args ...any) {
//	    a.l.Info(fmt.Sprintf(format, args...))
//	}
//
//	m := bucketfill.NewMigrator(client).WithLogger(adapter{l: myLogger})
type Logger interface {
	Logf(format string, args ...any)
}

// Migrator runs registered migrations against a bucket.
type Migrator struct {
	client *Client
	log    Logger // nil = print to os.Stdout
}

// NewMigrator builds a Migrator that operates via the given Client.
func NewMigrator(client *Client) *Migrator {
	return &Migrator{client: client}
}

// WithLogger returns a copy of m that writes progress through l.
// Pass nil to restore the default stdout logger.
func (m *Migrator) WithLogger(l Logger) *Migrator {
	cp := *m
	cp.log = l
	return &cp
}

func (m *Migrator) logf(format string, args ...any) {
	if m.log != nil {
		m.log.Logf(format, args...)
		return
	}
	if !strings.HasSuffix(format, "\n") {
		format += "\n"
	}
	fmt.Fprintf(os.Stdout, format, args...)
}

// Up applies every registered migration whose version is greater than the
// current state, in ascending order. State is persisted after each successful
// step so a mid-run failure leaves a consistent partial state.
//
// Returns an error if the registered versions have a gap (e.g. v1 and v3 with
// no v2) — likely a deleted file, and applying out of sequence is unsafe.
func (m *Migrator) Up(ctx context.Context) error {
	state, err := readState(ctx, m.client.storage, m.client.bucket)
	if err != nil {
		return err
	}

	all := Migrations()
	if err := checkContiguous(all); err != nil {
		return err
	}
	applied := 0

	for _, mig := range all {
		if mig.Version <= state.Version {
			continue
		}
		if mig.Up == nil {
			return fmt.Errorf("bucketfill: migration v%d has no Up function", mig.Version)
		}

		m.logf("bucketfill: applying v%d up...", mig.Version)
		c := m.client.WithData(mig.Data)
		if err := mig.Up(ctx, c); err != nil {
			return fmt.Errorf("bucketfill: v%d up failed: %w", mig.Version, err)
		}

		state.Version = mig.Version
		state.AppliedAt = time.Now().UTC()
		if err := writeState(ctx, m.client.storage, m.client.bucket, state); err != nil {
			return fmt.Errorf("bucketfill: v%d save state: %w", mig.Version, err)
		}
		m.logf("bucketfill: v%d up applied", mig.Version)
		applied++
	}

	if applied == 0 {
		m.logf("bucketfill: no pending migrations")
	}
	return nil
}

// Down rolls back the most recently applied migration.
func (m *Migrator) Down(ctx context.Context) error {
	state, err := readState(ctx, m.client.storage, m.client.bucket)
	if err != nil {
		return err
	}
	if state.Version == 0 {
		m.logf("bucketfill: no migrations to roll back")
		return nil
	}
	return m.DownTo(ctx, prevVersion(Migrations(), state.Version))
}

// DownTo rolls back applied migrations in reverse version order until
// state.Version <= target. target=0 rolls back everything.
func (m *Migrator) DownTo(ctx context.Context, target int) error {
	state, err := readState(ctx, m.client.storage, m.client.bucket)
	if err != nil {
		return err
	}
	if state.Version <= target {
		m.logf("bucketfill: already at v%d", state.Version)
		return nil
	}

	all := Migrations()
	if err := checkContiguous(all); err != nil {
		return err
	}

	for state.Version > target {
		mig := findVersion(all, state.Version)
		if mig == nil {
			return fmt.Errorf("bucketfill: migration v%d not found in registry", state.Version)
		}
		if mig.Down == nil {
			return fmt.Errorf("bucketfill: migration v%d has no Down function", state.Version)
		}

		m.logf("bucketfill: rolling back v%d...", mig.Version)
		c := m.client.WithData(mig.Data)
		if err := mig.Down(ctx, c); err != nil {
			return fmt.Errorf("bucketfill: v%d down failed: %w", mig.Version, err)
		}

		state.Version = prevVersion(all, mig.Version)
		state.AppliedAt = time.Now().UTC()
		if err := writeState(ctx, m.client.storage, m.client.bucket, state); err != nil {
			return fmt.Errorf("bucketfill: v%d save state: %w", mig.Version, err)
		}
		m.logf("bucketfill: v%d rolled back", mig.Version)
	}
	return nil
}

// Status prints the registered migrations and which are applied.
func (m *Migrator) Status(ctx context.Context) error {
	state, err := readState(ctx, m.client.storage, m.client.bucket)
	if err != nil {
		return err
	}
	all := Migrations()
	m.logf("Current version: %d", state.Version)
	m.logf("Registered migrations: %d", len(all))
	for _, mig := range all {
		status := "pending"
		if mig.Version <= state.Version {
			status = "applied"
		}
		m.logf("  v%d: %s", mig.Version, status)
	}
	return nil
}

func findVersion(all []*Migration, v int) *Migration {
	for _, m := range all {
		if m.Version == v {
			return m
		}
	}
	return nil
}

// prevVersion returns the largest registered version strictly less than v, or 0.
func prevVersion(all []*Migration, v int) int {
	prev := 0
	for _, m := range all {
		if m.Version < v && m.Version > prev {
			prev = m.Version
		}
	}
	return prev
}

// checkContiguous returns an error if registered versions have a gap.
// Versions must form 1, 2, 3, ... with no missing entries — a gap usually
// means a developer deleted a folder mid-history, and applying out of
// sequence past that gap is unsafe.
func checkContiguous(all []*Migration) error {
	for i, m := range all {
		want := i + 1
		if m.Version != want {
			return fmt.Errorf("bucketfill: gap in migration versions: expected v%d, found v%d", want, m.Version)
		}
	}
	return nil
}
