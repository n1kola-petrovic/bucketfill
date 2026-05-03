package bucketfill

import (
	"context"
	"fmt"
	"time"
)

// Migrator runs registered migrations against a bucket.
type Migrator struct {
	client *Client
}

// NewMigrator builds a Migrator that operates via the given Client.
func NewMigrator(client *Client) *Migrator {
	return &Migrator{client: client}
}

// Up applies every registered migration whose version is greater than the
// current state, in ascending order. State is persisted after each successful
// step so a mid-run failure leaves a consistent partial state.
func (m *Migrator) Up(ctx context.Context) error {
	state, err := readState(ctx, m.client.storage, m.client.bucket)
	if err != nil {
		return err
	}

	all := Migrations()
	applied := 0

	for _, mig := range all {
		if mig.Version <= state.Version {
			continue
		}
		if mig.Up == nil {
			return fmt.Errorf("bucketfill: migration v%d has no Up function", mig.Version)
		}

		fmt.Printf("bucketfill: applying v%d up...\n", mig.Version)
		c := m.client.WithData(mig.Data)
		if err := mig.Up(ctx, c); err != nil {
			return fmt.Errorf("bucketfill: v%d up failed: %w", mig.Version, err)
		}

		state.Version = mig.Version
		state.AppliedAt = time.Now().UTC()
		if err := writeState(ctx, m.client.storage, m.client.bucket, state); err != nil {
			return fmt.Errorf("bucketfill: v%d save state: %w", mig.Version, err)
		}
		fmt.Printf("bucketfill: v%d up applied\n", mig.Version)
		applied++
	}

	if applied == 0 {
		fmt.Println("bucketfill: no pending migrations")
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
		fmt.Println("bucketfill: no migrations to roll back")
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
		fmt.Printf("bucketfill: already at v%d\n", state.Version)
		return nil
	}

	all := Migrations()

	for state.Version > target {
		mig := findVersion(all, state.Version)
		if mig == nil {
			return fmt.Errorf("bucketfill: migration v%d not found in registry", state.Version)
		}
		if mig.Down == nil {
			return fmt.Errorf("bucketfill: migration v%d has no Down function", state.Version)
		}

		fmt.Printf("bucketfill: rolling back v%d...\n", mig.Version)
		c := m.client.WithData(mig.Data)
		if err := mig.Down(ctx, c); err != nil {
			return fmt.Errorf("bucketfill: v%d down failed: %w", mig.Version, err)
		}

		state.Version = prevVersion(all, mig.Version)
		state.AppliedAt = time.Now().UTC()
		if err := writeState(ctx, m.client.storage, m.client.bucket, state); err != nil {
			return fmt.Errorf("bucketfill: v%d save state: %w", mig.Version, err)
		}
		fmt.Printf("bucketfill: v%d rolled back\n", mig.Version)
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
	fmt.Printf("Current version: %d\n", state.Version)
	fmt.Printf("Registered migrations: %d\n", len(all))
	for _, mig := range all {
		status := "pending"
		if mig.Version <= state.Version {
			status = "applied"
		}
		fmt.Printf("  v%d: %s\n", mig.Version, status)
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
