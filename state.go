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

// readState reads the state file from the bucket. A missing object is treated
// as version 0. Providers must wrap their not-found errors with %w of
// os.ErrNotExist so this distinction works across fs / gcs / s3.
func readState(ctx context.Context, storage ObjectStorage, bucket string) (State, error) {
	rc, err := storage.Download(ctx, bucket, stateKey)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
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
