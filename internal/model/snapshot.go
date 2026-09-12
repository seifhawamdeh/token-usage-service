package model

import (
	"encoding/json"
	"time"
)

const SnapshotSchemaVersion = 2
const IdentityVersion = 1

// Tokens uses pointers so missing ≠ zero.
type Tokens struct {
	Input         *int64 `json:"input"`
	Output        *int64 `json:"output"`
	CacheRead     *int64 `json:"cache_read"`
	CacheWrite    *int64 `json:"cache_write"`     // total cache-write tokens when known
	CacheWrite5m  *int64 `json:"cache_write_5m"`  // Anthropic ephemeral 5m writes
	CacheWrite1h  *int64 `json:"cache_write_1h"`  // Anthropic ephemeral 1h writes
	Reasoning     *int64 `json:"reasoning"`
	Total         *int64 `json:"total"`
}

type BurnSnapshot struct {
	SourceID            string          `json:"source_id"`
	IdentityVersion     int             `json:"identity_version"`
	HostID              string          `json:"host_id"`
	Vendor              string          `json:"vendor"`
	SourcePath          string          `json:"source_path"`
	StableID            string          `json:"stable_id"`
	ProviderSessionID   string          `json:"provider_session_id,omitempty"`
	CWD                 string          `json:"cwd,omitempty"`
	StartedAt           *time.Time      `json:"started_at,omitempty"`
	LastEventAt         *time.Time      `json:"last_event_at,omitempty"`
	Model               string          `json:"model,omitempty"`
	Models              []string        `json:"models"`
	ModelProvider       string          `json:"model_provider,omitempty"`
	Tokens              Tokens          `json:"tokens"`
	ProviderCost        *float64        `json:"provider_cost,omitempty"`
	BillingRegime       string          `json:"billing_regime,omitempty"`
	UsageDetail         json.RawMessage `json:"usage_detail"`
	AdapterVersion      string          `json:"adapter_version"`
	SnapshotSchemaVer   int             `json:"snapshot_schema_version"`
	ParseStatus         string          `json:"parse_status"`
	IngestedAt          time.Time       `json:"ingested_at"`
}

type Checkpoint struct {
	SourceID             string
	HostID               string
	Vendor               string
	SourcePath           string
	MtimeNs              int64
	SizeBytes            int64
	ProcessingSignature  string
}

type PathRemote struct {
	HostID     string
	SourcePath string
	RemoteURL  *string
	RemoteName string
	RepoRoot   *string
}

type SourceDescriptor struct {
	Vendor     string
	SourcePath string
	StableID   string
	MtimeNs    int64
	SizeBytes  int64
}

type ParseResult struct {
	Snapshot *BurnSnapshot
	// Deferred means keep previous checkpoint/snapshot.
	Deferred bool
	// Skip means no usable usage (unsupported); do not write zero snapshot.
	Skip    bool
	Warning string
	Error   error
}
