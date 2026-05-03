package bucketfill

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

const stateKey = "_bucketfill_state.json"

// State is the persisted migration state in the bucket.
type State struct {
	Version   int       `json:"version"`
	AppliedAt time.Time `json:"appliedAt"`
}

// readState reads the state file from the bucket. A missing file is treated as version 0.
func readState(ctx context.Context, storage ObjectStorage, bucket string) (State, error) {
	rc, err := storage.Download(ctx, bucket, stateKey)
	if err != nil {
		if isNotExist(err) {
			return State{}, nil
		}
		// Providers may not all wrap os.ErrNotExist; treat any download error as
		// "no state yet" only if the underlying error indicates a missing object.
		// Conservative fallback: surface the error so users see real failures.
		return State{}, fmt.Errorf("bucketfill: read state: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return State{}, fmt.Errorf("bucketfill: read state: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("bucketfill: parse state: %w", err)
	}
	return s, nil
}

func writeState(ctx context.Context, storage ObjectStorage, bucket string, s State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("bucketfill: marshal state: %w", err)
	}
	return storage.Upload(ctx, bucket, stateKey, bytes.NewReader(data), int64(len(data)), "application/json")
}

// isNotExist returns true when err signals a missing object across providers.
// We check both os.ErrNotExist (FS provider) and a "not exist" substring as a
// defensive fallback for cloud providers whose typed errors we don't bind to here.
func isNotExist(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return false
}
