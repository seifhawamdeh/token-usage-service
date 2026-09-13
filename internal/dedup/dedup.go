// Package dedup clusters snapshots that likely represent the same underlying
// work (copies, resumes, branches, exports) so aggregates can count them once.
package dedup

import (
	"sort"

	"github.com/seif/token-usage-service/internal/model"
)

// timeWindowTolerance is how close two sessions' active windows must be
// (touching or overlapping, plus this slack) to count as the same session
// when no stronger signal (provider_session_id) is available.
const timeWindowTolerance = 10 * 60 // seconds, applied via time.Duration in Detect

const (
	confidenceProviderSessionID = 0.95
	confidenceTimeWindow        = 0.55
)

// Detect groups snapshots that are probably duplicates of each other.
// Two passes, strongest signal first:
//  1. Same vendor + same non-empty provider_session_id (copy/resume/export of
//     the same provider-tracked session, regardless of path).
//  2. Remaining snapshots: same host + same non-empty cwd with overlapping
//     (or near-overlapping) active time windows, across different source
//     paths — a weaker heuristic for cross-vendor or session-id-less
//     duplicates.
//
// Groups with fewer than 2 members are dropped. Within a group, the
// canonical (non-duplicate) snapshot is the one with the most complete
// token total, tie-broken by the latest last_event_at.
func Detect(snaps []model.BurnSnapshot) []model.DuplicateGroup {
	var groups []model.DuplicateGroup
	claimed := make(map[string]bool, len(snaps))

	groups = append(groups, groupByProviderSessionID(snaps, claimed)...)
	groups = append(groups, groupByTimeWindow(snaps, claimed)...)
	return groups
}

func groupByProviderSessionID(snaps []model.BurnSnapshot, claimed map[string]bool) []model.DuplicateGroup {
	type key struct{ vendor, sessionID string }
	buckets := make(map[key][]model.BurnSnapshot)
	for _, s := range snaps {
		if s.ProviderSessionID == "" {
			continue
		}
		k := key{s.Vendor, s.ProviderSessionID}
		buckets[k] = append(buckets[k], s)
	}

	var groups []model.DuplicateGroup
	for _, members := range buckets {
		if len(members) < 2 {
			continue
		}
		g := buildGroup("provider_session_id", confidenceProviderSessionID, members)
		groups = append(groups, g)
		for _, m := range members {
			claimed[m.SourceID] = true
		}
	}
	return groups
}

func groupByTimeWindow(snaps []model.BurnSnapshot, claimed map[string]bool) []model.DuplicateGroup {
	type key struct{ hostID, cwd string }
	buckets := make(map[key][]model.BurnSnapshot)
	for _, s := range snaps {
		if claimed[s.SourceID] || s.CWD == "" {
			continue
		}
		if s.StartedAt == nil && s.LastEventAt == nil {
			continue
		}
		k := key{s.HostID, s.CWD}
		buckets[k] = append(buckets[k], s)
	}

	var groups []model.DuplicateGroup
	for _, members := range buckets {
		sort.Slice(members, func(i, j int) bool {
			return windowStart(members[i]) < windowStart(members[j])
		})

		var cluster []model.BurnSnapshot
		clusterEnd := int64(0)
		flush := func() {
			if len(cluster) < 2 {
				cluster = nil
				return
			}
			// Require at least two distinct source paths — same path twice
			// isn't a duplicate, it's the same source re-ingested.
			paths := make(map[string]bool, len(cluster))
			for _, m := range cluster {
				paths[m.SourcePath] = true
			}
			if len(paths) < 2 {
				cluster = nil
				return
			}
			g := buildGroup("time_window", confidenceTimeWindow, cluster)
			groups = append(groups, g)
			for _, m := range cluster {
				claimed[m.SourceID] = true
			}
			cluster = nil
		}

		for _, m := range members {
			start := windowStart(m)
			end := windowEnd(m)
			if len(cluster) == 0 {
				cluster = append(cluster, m)
				clusterEnd = end
				continue
			}
			if start <= clusterEnd+timeWindowTolerance {
				cluster = append(cluster, m)
				if end > clusterEnd {
					clusterEnd = end
				}
				continue
			}
			flush()
			cluster = append(cluster, m)
			clusterEnd = end
		}
		flush()
	}
	return groups
}

func windowStart(s model.BurnSnapshot) int64 {
	if s.StartedAt != nil {
		return s.StartedAt.Unix()
	}
	return s.LastEventAt.Unix()
}

func windowEnd(s model.BurnSnapshot) int64 {
	if s.LastEventAt != nil {
		return s.LastEventAt.Unix()
	}
	return s.StartedAt.Unix()
}

func buildGroup(basis string, confidence float64, members []model.BurnSnapshot) model.DuplicateGroup {
	ids := make([]string, len(members))
	canonical := members[0]
	for i, m := range members {
		ids[i] = m.SourceID
		if better(m, canonical) {
			canonical = m
		}
	}
	return model.DuplicateGroup{
		Basis:             basis,
		Confidence:        confidence,
		CanonicalSourceID: canonical.SourceID,
		SourceIDs:         ids,
	}
}

// better reports whether a is a stronger canonical candidate than b: more
// complete token total wins, ties broken by the more recent last_event_at.
func better(a, b model.BurnSnapshot) bool {
	at, bt := tokenTotal(a), tokenTotal(b)
	if at != bt {
		return at > bt
	}
	ae, be := a.LastEventAt, b.LastEventAt
	if ae == nil {
		return false
	}
	if be == nil {
		return true
	}
	return ae.After(*be)
}

func tokenTotal(s model.BurnSnapshot) int64 {
	if s.Tokens.Total == nil {
		return -1
	}
	return *s.Tokens.Total
}
