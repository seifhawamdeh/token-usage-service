package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/seif/token-usage-service/internal/adapter"
	"github.com/seif/token-usage-service/internal/identity"
	"github.com/seif/token-usage-service/internal/model"
	"github.com/seif/token-usage-service/internal/pathremote"
	"github.com/seif/token-usage-service/internal/sink"
)

type Summary struct {
	Scanned           int
	UnchangedSkipped  int
	Parsed            int
	Upserted          int
	Deferred          int
	Errors            int
	PathRemotesUpsert int
}

type Runner struct {
	HostID   string
	Adapters []adapter.VendorAdapter
	Sink     sink.Sink
	Force    bool
	Log      *slog.Logger
}

func (r *Runner) Run(ctx context.Context) (Summary, error) {
	log := r.Log
	if log == nil {
		log = slog.Default()
	}
	var sum Summary

	for _, ad := range r.Adapters {
		sig := identity.ProcessingSignature(ad.Name(), ad.Version(), model.SnapshotSchemaVersion)
		sources, err := ad.Discover(ctx)
		if err != nil {
			return sum, fmt.Errorf("discover %s: %w", ad.Name(), err)
		}
		paths := make([]string, len(sources))
		for i, s := range sources {
			paths[i] = s.SourcePath
		}
		cps, err := r.Sink.LookupCheckpoints(ctx, r.HostID, ad.Name(), paths)
		if err != nil {
			return sum, fmt.Errorf("checkpoints %s: %w", ad.Name(), err)
		}

		for _, src := range sources {
			sum.Scanned++
			cp, ok := cps[src.SourcePath]
			if !r.Force && ok && cp.MtimeNs == src.MtimeNs && cp.SizeBytes == src.SizeBytes && cp.ProcessingSignature == sig {
				sum.UnchangedSkipped++
				continue
			}

			res := ad.Parse(ctx, src)
			if res.Error != nil {
				sum.Errors++
				log.Error("parse error", "vendor", ad.Name(), "path", src.SourcePath, "err", res.Error)
				continue
			}
			if res.Deferred {
				sum.Deferred++
				log.Warn("deferred", "vendor", ad.Name(), "path", src.SourcePath, "warning", res.Warning)
				continue
			}
			if res.Skip || res.Snapshot == nil {
				sum.Errors++
				log.Warn("skip snapshot", "vendor", ad.Name(), "path", src.SourcePath, "warning", res.Warning)
				continue
			}
			sum.Parsed++

			snap := *res.Snapshot
			snap.HostID = r.HostID
			snap.IdentityVersion = model.IdentityVersion
			snap.SourceID = identity.SourceID(r.HostID, ad.Name(), src.SourcePath)
			snap.Vendor = ad.Name()
			snap.SourcePath = src.SourcePath
			if snap.StableID == "" {
				snap.StableID = src.StableID
			}
			snap.AdapterVersion = ad.Version()
			snap.SnapshotSchemaVer = model.SnapshotSchemaVersion
			if snap.ParseStatus == "" {
				snap.ParseStatus = "ok"
			}
			snap.IngestedAt = time.Now().UTC()
			if snap.Models == nil {
				snap.Models = []string{}
			}

			newCP := model.Checkpoint{
				SourceID:            snap.SourceID,
				HostID:              r.HostID,
				Vendor:              ad.Name(),
				SourcePath:          src.SourcePath,
				MtimeNs:             src.MtimeNs,
				SizeBytes:           src.SizeBytes,
				ProcessingSignature: sig,
			}

			if err := r.Sink.UpsertSnapshotAndCheckpoint(ctx, snap, newCP); err != nil {
				return sum, fmt.Errorf("sink upsert: %w", err)
			}
			sum.Upserted++

			pr := pathremote.ResolveFromCWD(r.HostID, src.SourcePath, snap.CWD)
			if err := r.Sink.UpsertPathRemote(ctx, pr); err != nil {
				return sum, fmt.Errorf("path remote: %w", err)
			}
			sum.PathRemotesUpsert++
		}
	}
	return sum, nil
}
