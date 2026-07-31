// 与后端契约一致的类型定义（01_API_v1 冻结信封与数据结构）

export interface Envelope<T = unknown> {
  code: number
  message: string
  data?: T
  request_id: string
  details?: { field: string; issue: string; existing_repo_id?: string }[]
}

export interface Progress { current: number; total: number; percent: number }
export interface Stats { files: number; chunks: number; tokens: number }
export interface TaskError { code: number; message: string; stage: string }

export interface Task {
  task_id: string
  type: 'ingest' | 'refresh' | 'wiki'
  repo_id: string
  state: string
  progress: Progress
  stats: Stats
  error: TaskError | null
  queue_position: number
  created_at: string
  started_at?: string | null
  finished_at?: string | null
}

export interface Repo {
  repo_id: string
  repo_url: string
  branch: string
  commit_hash: string
  state: 'ingesting' | 'ready' | 'error'
  stats: Stats
  created_at: string
  updated_at: string
  latest_task?: Task
  wiki_available?: boolean
  chunk_count?: number
}

export interface Pagination { page: number; page_size: number; total: number; total_pages: number }
export interface PageResult<T> { items: T[] | null; pagination: Pagination }

export interface TaskSubmitted {
  task_id: string
  repo_id: string
  type: string
  state: string
  queue_position: number
  created_at: string
}

export interface Reference {
  chunk_id: string
  path: string
  start_line: number
  end_line: number
  language: string
  score: number
  snippet: string
}

export interface AskResponse {
  answer: string
  references: Reference[]
  mode: string
  usage: { prompt_tokens: number; completion_tokens: number }
  latency_ms: number
}

export interface WikiTOCItem { slug: string; title: string; parent_slug: string; sort_order: number }
export interface WikiPage { slug: string; title: string; content_md: string; sort_order: number; updated_at: string }
export interface Wiki {
  repo_id: string
  toc: WikiTOCItem[]
  pages: WikiPage[]
  task_id: string
  generated_at: string
}

export interface Health {
  status: string
  version: string
  llm: { provider: string; model: string; reachable: boolean; breaker: string }
  embedding: { provider: string; model: string; reachable: boolean; breaker: string; dimensions?: number }
  postgres: { connected: boolean }
  opensearch: { connected: boolean; cluster_status?: string; indices?: number }
  rabbitmq: { connected: boolean; queue_depth: number; consumers?: number }
  redis: { connected: boolean; ratelimit_degraded: boolean }
  etcd: { connected: boolean }
  git: { available: boolean }
  worker: { busy: number; total: number; queued: number }
}

export interface DWEvent {
  seq: number
  type: string
  repo_id: string
  task_id: string
  timestamp: string
  payload: { state?: string; progress?: Progress; stats?: Stats; queue_position?: number }
}
