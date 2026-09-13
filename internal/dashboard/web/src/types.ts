export interface Summary {
  sources: number;
  sources_non_cursor: number;
  tokens_input: number;
  tokens_output: number;
  tokens_cache_read: number;
  tokens_cache_write: number;
  tokens_cache_write_5m: number;
  tokens_cache_write_1h: number;
  rated_cost_usd: number;
  provider_cost_sum: number;
  sources_with_rated_cost: number;
  first_event_at: string | null;
  last_event_at: string | null;
}

export interface Vendor {
  vendor: string;
  sources: number;
  tokens_input: number;
  tokens_output: number;
  tokens_cache_read: number;
  tokens_cache_write: number;
  rated_cost_usd: number;
  sources_with_rated_cost: number;
}

export interface Model {
  model: string;
  vendor: string;
  sources: number;
  tokens_input: number;
  tokens_output: number;
  rated_cost_usd: number;
}

export interface Project {
  project: string;
  sources: number;
  tokens_input: number;
  tokens_output: number;
  tokens_cache_read: number;
  tokens_cache_write: number;
  rated_cost_usd: number;
  sources_with_rated_cost: number;
}

export interface Daily {
  day: string;
  tokens_input: number;
  tokens_output: number;
  rated_cost_usd: number;
  sources: number;
}

export interface Snapshot {
  source_id: string;
  machine: string;
  vendor: string;
  model: string;
  source_path: string;
  project: string;
  tokens_input: number | null;
  tokens_output: number | null;
  tokens_cache_read: number | null;
  tokens_cache_write: number | null;
  tokens_cache_write_5m: number | null;
  tokens_cache_write_1h: number | null;
  rated_cost_usd: number | null;
  provider_cost: number | null;
  last_event_at: string | null;
  started_at: string | null;
  ingested_at: string;
}

export interface Remote {
  remote_url: string;
  project: string | null;
  snapshots: number;
}

export interface Cwd {
  host_id: string;
  cwd: string;
  project: string | null;
  snapshots: number;
}

export interface Machine {
  machine: string;
  sources: number;
  last_sync_at: string | null;
}
