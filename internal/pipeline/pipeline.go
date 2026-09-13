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
	Skipped           int
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

func (r *Runner) Run(ctx context.Context) (sum Summary, runErr error) {
	log := r.Log
	if log == nil {
		log = slog.Default()
	}
	var recorder sink.RunRecorder
	var runID int64
	if candidate, ok := r.Sink.(sink.RunRecorder); ok {
		recorder = candidate
		var err error
		runID, err = recorder.StartRun(ctx, r.HostID)
		if err != nil {
			return sum, fmt.Errorf("start ingest run: %w", err)
		}
		defer func() {
			status := "ok"
			fatal := ""
			if runErr != nil {
				status, fatal = "failed", runErr.Error()
			} else if sum.Errors > 0 || sum.Deferred > 0 {
				status = "degraded"
			}
			err := recorder.FinishRun(ctx, runID, model.IngestRun{
				HostID: r.HostID, Status: status, Scanned: sum.Scanned, UnchangedSkipped: sum.UnchangedSkipped,
				Parsed: sum.Parsed, Upserted: sum.Upserted, Skipped: sum.Skipped, Deferred: sum.Deferred, Errors: sum.Errors,
				PathRemotesUpsert: sum.PathRemotesUpsert, FatalError: fatal,
			})
			if err != nil {
				log.Error("finish ingest run", "run_id", runID, "err", err)
				if runErr == nil {
					runErr = fmt.Errorf("finish ingest run: %w", err)
				}
			}
		}()
	}

	for _, ad := range r.Adapters {
		sig := identity.ProcessingSignature(ad.Name(), ad.Version(), model.SnapshotSchemaVersion)
		sources, err := ad.Discover(ctx)
		if err != nil {
			recordIssue(ctx, recorder, runID, model.IngestIssue{Vendor: ad.Name(), Severity: "error", Message: "discover: " + err.Error()}, log)
			return sum, fmt.Errorf("discover %s: %w", ad.Name(), err)
		}
		paths := make([]string, len(sources))
		for i, s := range sources {
			paths[i] = s.SourcePath
		}
		cps, err := r.Sink.LookupCheckpoints(ctx, r.HostID, ad.Name(), paths)
		if err != nil {
			recordIssue(ctx, recorder, runID, model.IngestIssue{Vendor: ad.Name(), Severity: "error", Message: "lookup checkpoints: " + err.Error()}, log)
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
				recordIssue(ctx, recorder, runID, model.IngestIssue{Vendor: ad.Name(), SourcePath: src.SourcePath, Severity: "error", Message: res.Error.Error()}, log)
				continue
			}
			if res.Deferred {
				sum.Deferred++
				log.Warn("deferred", "vendor", ad.Name(), "path", src.SourcePath, "warning", res.Warning)
				recordIssue(ctx, recorder, runID, model.IngestIssue{Vendor: ad.Name(), SourcePath: src.SourcePath, Severity: "warning", Message: res.Warning}, log)
				continue
			}
			if res.Skip || res.Snapshot == nil {
				sum.Skipped++
				log.Warn("skip snapshot", "vendor", ad.Name(), "path", src.SourcePath, "warning", res.Warning)
				recordIssue(ctx, recorder, runID, model.IngestIssue{Vendor: ad.Name(), SourcePath: src.SourcePath, Severity: "warning", Message: res.Warning}, log)
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
				recordIssue(ctx, recorder, runID, model.IngestIssue{Vendor: ad.Name(), SourcePath: src.SourcePath, Severity: "error", Message: "snapshot upsert: " + err.Error()}, log)
				return sum, fmt.Errorf("sink upsert: %w", err)
			}
			sum.Upserted++

			pr := pathremote.ResolveWithKnownRemote(r.HostID, src.SourcePath, snap.CWD, snap.GitRemoteURL)
			if err := r.Sink.UpsertPathRemote(ctx, pr); err != nil {
				recordIssue(ctx, recorder, runID, model.IngestIssue{Vendor: ad.Name(), SourcePath: src.SourcePath, Severity: "error", Message: "path remote: " + err.Error()}, log)
				return sum, fmt.Errorf("path remote: %w", err)
			}
			sum.PathRemotesUpsert++
		}
	}
	return sum, nil
}

func recordIssue(ctx context.Context, recorder sink.RunRecorder, runID int64, issue model.IngestIssue, log *slog.Logger) {
	if recorder == nil {
		return
	}
	if err := recorder.RecordRunIssue(ctx, runID, issue); err != nil {
		log.Error("record ingest issue", "run_id", runID, "err", err)
	}
}
