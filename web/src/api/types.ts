export interface PluginConfig {
  type: string
  pattern: string
  replacement: string
  template?: string
  match_mode?: string
  filter_mode?: string
  directory_only?: boolean
  case_sensitive?: boolean
  delay_ms?: number
  max_bytes?: number
}

export interface TaskConfig {
  id: string
  name: string
  storage_id: string
  source: string
  destination: string
  schedule: string
  file_id_mode: boolean
  dir_time_check: boolean
  watch: boolean
  incremental: boolean
  sync_delete: boolean
  media_ext: string[]
  copy_ext: string[]
  min_size: number
  max_size: number
  plugins: PluginConfig[]
  keep_local: string[]
}

export interface TaskResult {
  scanned?: number
  created?: number
  copied?: number
  removed?: number
  skipped?: number
  failed?: number
}

export interface TaskStatus {
  task_id: string
  running: boolean
  queued: number
  started_at?: string
  ended_at?: string
  result: TaskResult
  error?: string
  stopped?: boolean
}

export interface TasksResponse {
  tasks: TaskConfig[]
  statuses: TaskStatus[]
}

export interface StorageSummary {
  id: string
  type: string
  root: string
}

export interface StorageEntry {
  path: string
  name: string
  size: number
  is_dir: boolean
  modified: string
}

export interface MatchingTask {
  id: string
  name: string
}

export interface HistoryEvent {
  id: number
  timestamp: string
  task_id: string
  type: string
  path?: string
  created?: number
  copied?: number
  removed?: number
  skipped?: number
  failed?: number
  error?: string
}

export interface StorageConfig {
  id: string
  type: string
  root: string
  endpoint: string
  username: string
  password: string
  token: string
}

export interface ManagedConfig {
  storages: StorageConfig[]
  tasks: TaskConfig[]
  plugins: PluginConfig[]
  integrations: {
    clouddrive2: Record<string, string>
    moviepilot: Record<string, string>
  }
  tmdb: {
    api_key: string
    language: string
    cache_minutes?: number
    base_url?: string
    image_base_url?: string
    proxy_url?: string
  }
  media_tools: { ffmpeg: string; timeout_seconds: number }
  history: { path: string; max_entries: number }
  media_proxy: {
    enabled: boolean
    listen: string
    upstream: string
    server_type: string
    rewrite_playback_info: boolean
  }
}

export interface Bootstrap {
  tmdb_configured: boolean
}

export interface Overview {
  tasks: { total: number; running: number; failed: number; queued: number }
  storages: number
  history: HistoryEvent[]
}
