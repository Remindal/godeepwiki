import type { Envelope } from './types'

export class ApiError extends Error {
  constructor(
    public code: number,
    message: string,
    public details?: Envelope['details'],
  ) {
    super(message)
  }
}

function apiKey(): string {
  return localStorage.getItem('dw_api_key') || ''
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const key = apiKey()
  if (key) headers['X-API-Key'] = key

  const resp = await fetch(url, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  const env = (await resp.json()) as Envelope<T>
  if (env.code !== 0) {
    throw new ApiError(env.code, env.message, env.details)
  }
  return env.data as T
}

export const api = {
  get: <T>(url: string) => request<T>('GET', url),
  post: <T>(url: string, body?: unknown) => request<T>('POST', url, body),
  put: <T>(url: string, body?: unknown) => request<T>('PUT', url, body),
  del: <T>(url: string) => request<T>('DELETE', url),
}

// ---------------- SSE 解析（fetch ReadableStream，POST 也能用） ----------------

export interface SSEFrame {
  id?: string
  event: string
  data: string
}

// streamSSE 解析 text/event-stream；onFrame 逐帧回调；返回中断函数。
export function streamSSE(
  url: string,
  opts: { method?: string; body?: unknown; headers?: Record<string, string> },
  onFrame: (f: SSEFrame) => void,
): { abort: () => void; done: Promise<void> } {
  const ctrl = new AbortController()
  const headers: Record<string, string> = { ...(opts.headers || {}) }
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json'
  const key = apiKey()
  if (key) headers['X-API-Key'] = key

  const done = (async () => {
    const resp = await fetch(url, {
      method: opts.method || 'GET',
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      signal: ctrl.signal,
    })
    if (!resp.ok || !resp.body) {
      let msg = `http ${resp.status}`
      try {
        const env = (await resp.json()) as Envelope
        msg = env.message || msg
        throw new ApiError(env.code, msg, env.details)
      } catch (e) {
        if (e instanceof ApiError) throw e
        throw new Error(msg)
      }
    }
    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buf = ''
    for (;;) {
      const { done: finished, value } = await reader.read()
      if (finished) break
      buf += decoder.decode(value, { stream: true })
      let idx: number
      // SSE 帧以空行分隔
      while ((idx = buf.indexOf('\n\n')) >= 0) {
        const raw = buf.slice(0, idx)
        buf = buf.slice(idx + 2)
        const frame = parseFrame(raw)
        if (frame) onFrame(frame)
      }
    }
  })()

  return { abort: () => ctrl.abort(), done }
}

function parseFrame(raw: string): SSEFrame | null {
  if (!raw.trim() || raw.startsWith(':')) return null // 心跳/注释
  const frame: SSEFrame = { event: 'message', data: '' }
  for (const line of raw.split('\n')) {
    if (line.startsWith('id:')) frame.id = line.slice(3).trim()
    else if (line.startsWith('event:')) frame.event = line.slice(6).trim()
    else if (line.startsWith('data:')) frame.data += (frame.data ? '\n' : '') + line.slice(5).trim()
  }
  return frame.data || frame.event !== 'message' ? frame : null
}
