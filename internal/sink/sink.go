package sink

import (
	"context"

	"github.com/seif/token-usage-service/internal/model"
)

type Sink interface {
	Name() string
	Close() error
	// LookupCheckpoints returns known checkpoints keyed by source_path for the vendor.
	LookupCheckpoints(ctx context.Context, hostID, vendor string, paths []string) (map[string]model.Checkpoint, error)
	// UpsertSnapshotAndCheckpoint atomically replaces snapshot + checkpoint.
	UpsertSnapshotAndCheckpoint(ctx context.Context, snap model.BurnSnapshot, cp model.Checkpoint) error
	// UpsertPathRemote writes the path↔remote mapping (separate table).
	UpsertPathRemote(ctx context.Context, pr model.PathRemote) error
}
