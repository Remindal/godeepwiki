import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { streamSSE, ApiError } from '../api/client'
import type { Reference } from '../api/types'

export interface ChatMsg {
  role: 'user' | 'ai'
  content: string
  thinking?: string
  thinkingOpen?: boolean
  streaming?: boolean
  references?: Reference[]
  refsOpen?: boolean
  usage?: { prompt_tokens: number; completion_tokens: number }
  latency_ms?: number
  startedAt?: number
}

export interface ChatSession {
  messages: ChatMsg[]
  streaming: boolean
  abort?: () => void
}

export interface ConvMeta {
  id: string
  title: string
  updatedAt: number
}

// historiesVersion 历史列表版本号：保存/清空/新建/删除会话时递增，侧栏据此刷新。
export const historiesVersion = ref(0)

const sessions = reactive(new Map<string, ChatSession>())
const CHAT_MAX = 50
const DEFAULT_CONV = 'default'

const convsKey = (repoId: string) => `dw_convs_${repoId}`
const chatKey = (repoId: string, convId: string) =>
  convId === DEFAULT_CONV ? `dw_chat_${repoId}` : `dw_chat_${repoId}_${convId}`

// ---------------- 会话索引（每仓库一份） ----------------

function loadConvs(repoId: string): ConvMeta[] {
  try {
    const raw = localStorage.getItem(convsKey(repoId))
    if (raw) {
      const list = JSON.parse(raw) as ConvMeta[]
      if (Array.isArray(list)) return list
    }
  } catch { /* fallthrough */ }
  // 迁移：旧版单会话 dw_chat_<repoId> 存在时补一条默认会话索引。
  if (localStorage.getItem(chatKey(repoId, DEFAULT_CONV))) {
    return [{ id: DEFAULT_CONV, title: '默认对话', updatedAt: Date.now() }]
  }
  return []
}

function saveConvs(repoId: string, convs: ConvMeta[]) {
  localStorage.setItem(convsKey(repoId), JSON.stringify(convs))
  historiesVersion.value++
}

export function listConversations(repoId: string): ConvMeta[] {
  const convs = loadConvs(repoId)
  if (!convs.length) {
    const meta: ConvMeta = { id: DEFAULT_CONV, title: '默认对话', updatedAt: Date.now() }
    saveConvs(repoId, [meta])
    return [meta]
  }
  return convs.slice().sort((a, b) => b.updatedAt - a.updatedAt)
}

export function createConversation(repoId: string): ConvMeta {
  const convs = loadConvs(repoId)
  const meta: ConvMeta = {
    id: `c${Date.now().toString(36)}`,
    title: '新对话',
    updatedAt: Date.now(),
  }
  convs.push(meta)
  saveConvs(repoId, convs)
  return meta
}

export function deleteConversation(repoId: string, convId: string) {
  const s = sessions.get(`${repoId}:${convId}`)
  s?.abort?.()
  sessions.delete(`${repoId}:${convId}`)
  localStorage.removeItem(chatKey(repoId, convId))
  const convs = loadConvs(repoId).filter((c) => c.id !== convId)
  saveConvs(repoId, convs)
}

function touchConversation(repoId: string, convId: string, title?: string) {
  const convs = loadConvs(repoId)
  const meta = convs.find((c) => c.id === convId)
  if (meta) {
    meta.updatedAt = Date.now()
    if (title && (meta.title === '新对话' || meta.title === '默认对话')) meta.title = title
  } else {
    convs.push({ id: convId, title: title ?? '新对话', updatedAt: Date.now() })
  }
  saveConvs(repoId, convs)
}

// ---------------- 消息持久化 ----------------

function loadMessages(repoId: string, convId: string): ChatMsg[] {
  try {
    const raw = localStorage.getItem(chatKey(repoId, convId))
    if (!raw) return []
    const parsed = JSON.parse(raw)
    const list: ChatMsg[] = Array.isArray(parsed) ? parsed : parsed?.messages ?? []
    return Array.isArray(list) ? list : []
  } catch {
    return []
  }
}

function saveMessages(repoId: string, convId: string, list: ChatMsg[]) {
  try {
    const finalized = list
      .filter((m) => !m.streaming)
      .slice(-CHAT_MAX)
      .map((m) => ({
        role: m.role,
        content: m.content,
        thinking: m.thinking,
        references: m.references,
        usage: m.usage,
        latency_ms: m.latency_ms,
      }))
    localStorage.setItem(
      chatKey(repoId, convId),
      JSON.stringify({ updatedAt: Date.now(), messages: finalized }),
    )
    const firstUser = finalized.find((m) => m.role === 'user')
    touchConversation(repoId, convId, firstUser?.content?.slice(0, 30))
  } catch { /* 存储满等异常静默 */ }
}

// ---------------- 会话存取 ----------------

export function getChat(repoId: string, convId: string = DEFAULT_CONV): ChatSession {
  const key = `${repoId}:${convId}`
  let s = sessions.get(key)
  if (!s) {
    s = reactive<ChatSession>({ messages: loadMessages(repoId, convId), streaming: false })
    sessions.set(key, s)
  }
  return s
}

export function clearChat(repoId: string, convId: string = DEFAULT_CONV) {
  const s = getChat(repoId, convId)
  s.abort?.()
  s.messages = []
  s.streaming = false
  localStorage.removeItem(chatKey(repoId, convId))
  touchConversation(repoId, convId)
}

// ---------------- 侧栏历史索引 ----------------

export interface ChatHistoryEntry {
  repoId: string
  convId: string
  preview: string
  updatedAt: number
}

// listChatHistories 扫描全部仓库的会话索引（dw_convs_*），按最近更新倒序。
export function listChatHistories(): ChatHistoryEntry[] {
  const out: ChatHistoryEntry[] = []
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i)
    if (!key || !key.startsWith('dw_convs_')) continue
    try {
      const convs = JSON.parse(localStorage.getItem(key) || '[]') as ConvMeta[]
      const repoId = key.slice('dw_convs_'.length)
      for (const c of convs) {
        out.push({ repoId, convId: c.id, preview: c.title, updatedAt: c.updatedAt })
      }
    } catch { /* 跳过坏数据 */ }
  }
  out.sort((a, b) => b.updatedAt - a.updatedAt)
  return out
}

// ---------------- 流式问答 ----------------

function typewriterPacer(ai: ChatMsg, onUpdate?: () => void) {
  let pending = ''
  let timer: number | undefined
  const push = (delta: string) => {
    pending += delta
    if (timer === undefined) {
      timer = window.setInterval(() => {
        if (pending.length === 0) {
          clearInterval(timer)
          timer = undefined
          return
        }
        const n = Math.min(2, pending.length)
        ai.content += pending.slice(0, n)
        pending = pending.slice(n)
        onUpdate?.()
      }, 30)
    }
  }
  const flush = () => {
    if (pending.length > 0) {
      ai.content += pending
      pending = ''
      onUpdate?.()
    }
  }
  return { push, flush }
}

export function stopChat(repoId: string, convId: string = DEFAULT_CONV) {
  getChat(repoId, convId).abort?.()
}

export async function ask(
  repoId: string,
  convId: string,
  question: string,
  mode: string,
  pathFilter: string,
  onUpdate?: () => void,
): Promise<void> {
  const s = getChat(repoId, convId)
  const q = question.trim()
  if (!q || s.streaming) return

  const history = s.messages
    .filter((m) => !m.streaming)
    .slice(-6)
    .map((m) => ({ role: m.role === 'ai' ? 'assistant' : 'user', content: m.content }))

  s.messages.push({ role: 'user', content: q })
  s.messages.push({
    role: 'ai', content: '', thinking: '', streaming: true, refsOpen: false, thinkingOpen: false,
    startedAt: Date.now(),
  })
  // 必须经响应式代理访问（数组下标经 Proxy 包装），否则流式变更不触发重渲染。
  const ai = s.messages[s.messages.length - 1]
  s.streaming = true
  saveMessages(repoId, convId, s.messages)
  onUpdate?.()

  const pacer = typewriterPacer(ai, onUpdate)
  const body: Record<string, unknown> = { repo_id: repoId, question: q, mode, top_k: 8, history }
  if (pathFilter.trim()) body.path_filter = pathFilter.trim()

  const { abort, done } = streamSSE(
    '/api/v1/ask/stream',
    { method: 'POST', body },
    (frame) => {
      try {
        const payload = JSON.parse(frame.data)
        if (frame.event === 'references') {
          ai.references = payload.references
          onUpdate?.()
        } else if (frame.event === 'thinking') {
          ai.thinking = (ai.thinking || '') + payload.delta
          onUpdate?.()
        } else if (frame.event === 'token') {
          pacer.push(payload.delta)
        } else if (frame.event === 'done') {
          pacer.flush()
          ai.usage = payload.usage
          ai.latency_ms = payload.latency_ms
          ai.streaming = false
          onUpdate?.()
        } else if (frame.event === 'error') {
          pacer.flush()
          ai.content = `出错了：${payload.message}`
          ai.streaming = false
          onUpdate?.()
        }
      } catch { /* 忽略非 JSON 帧 */ }
    },
  )
  s.abort = abort

  try {
    await done
  } catch (e) {
    ai.streaming = false
    if ((e as Error).name === 'AbortError') {
      // 显式停止/离开：静默收尾，保留已生成内容。
    } else if (e instanceof ApiError) {
      ai.content = `出错了：${e.message}`
      ElMessage.error(ai.content)
    } else {
      ai.content = '连接中断，请重试'
      ElMessage.error(ai.content)
    }
  } finally {
    s.streaming = false
    s.abort = undefined
    saveMessages(repoId, convId, s.messages)
    onUpdate?.()
  }
}
