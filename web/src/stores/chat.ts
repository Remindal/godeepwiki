import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { streamSSE, ApiError } from '../api/client'
import type { Reference } from '../api/types'

// historiesVersion 历史列表版本号：saveHistory 时递增，侧栏 watch 它刷新「历史对话」栏目。
export const historiesVersion = ref(0)

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
  startedAt?: number // 流式开始时刻（“已思考 N 秒”展示用）
}

export interface ChatSession {
  messages: ChatMsg[]
  streaming: boolean
  abort?: () => void
}

// 会话仓库：模块级单例，不随路由组件销毁——离开页面流式继续在后台跑（市面 AI 对话行为）。
const sessions = reactive(new Map<string, ChatSession>())

const CHAT_MAX = 50
const chatKey = (repoId: string) => `dw_chat_${repoId}`

function loadHistory(repoId: string): ChatMsg[] {
  try {
    const raw = localStorage.getItem(chatKey(repoId))
    if (!raw) return []
    const parsed = JSON.parse(raw)
    // 兼容两种格式：{updatedAt, messages} 与旧版纯数组。
    const list: ChatMsg[] = Array.isArray(parsed) ? parsed : parsed?.messages ?? []
    return Array.isArray(list) ? list : []
  } catch {
    return []
  }
}

function saveHistory(repoId: string, list: ChatMsg[]) {
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
      chatKey(repoId),
      JSON.stringify({ updatedAt: Date.now(), messages: finalized }),
    )
    historiesVersion.value++
  } catch { /* 存储满等异常静默 */ }
}

// ChatHistoryEntry 侧栏「历史对话」条目。
export interface ChatHistoryEntry {
  repoId: string
  preview: string // 首条用户消息（截断）
  count: number
  updatedAt: number
}

// listChatHistories 扫描 localStorage 中的全部会话（dw_chat_*），按最近更新倒序。
export function listChatHistories(): ChatHistoryEntry[] {
  const out: ChatHistoryEntry[] = []
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i)
    if (!key || !key.startsWith('dw_chat_')) continue
    try {
      const raw = JSON.parse(localStorage.getItem(key) || '')
      const msgs: ChatMsg[] = Array.isArray(raw) ? raw : raw?.messages ?? []
      if (!msgs.length) continue
      const firstUser = msgs.find((m) => m.role === 'user')
      out.push({
        repoId: key.slice('dw_chat_'.length),
        preview: (firstUser?.content ?? '').slice(0, 40),
        count: msgs.length,
        updatedAt: Array.isArray(raw) ? 0 : raw.updatedAt ?? 0,
      })
    } catch { /* 跳过坏数据 */ }
  }
  out.sort((a, b) => b.updatedAt - a.updatedAt)
  return out
}

export function getChat(repoId: string): ChatSession {
  let s = sessions.get(repoId)
  if (!s) {
    s = reactive<ChatSession>({ messages: loadHistory(repoId), streaming: false })
    sessions.set(repoId, s)
  }
  return s
}

export function clearChat(repoId: string) {
  const s = getChat(repoId)
  s.abort?.()
  s.messages = []
  s.streaming = false
  localStorage.removeItem(chatKey(repoId))
  historiesVersion.value++
}

// typewriterPacer 打字机节流：provider 常常「沉默思考十几秒 → 几百字一秒内灌完」，
// 直接渲染等于「一下全弹出」。把到达的 delta 入队，按 ~66 字/秒匀速放出，
// 体感接近市面 AI 的逐字输出。timer 挂 window，与组件生命周期无关。
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

// ask 在会话仓库层发起流式请求；调用方（组件）只负责渲染与滚动。
// onUpdate 在每帧后回调（组件用来滚到底部），组件卸载后传 null 也不影响后台续跑。
export async function ask(
  repoId: string,
  question: string,
  mode: string,
  onUpdate?: () => void,
): Promise<void> {
  const s = getChat(repoId)
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
  // 必须经响应式代理访问（数组下标经 Proxy 包装）：直接改原始对象不会触发重渲染，
  // 曾导致“停留在思考中、切走再回来才显示”的问题。
  const ai = s.messages[s.messages.length - 1]
  s.streaming = true
  saveHistory(repoId, s.messages)
  onUpdate?.()

  const pacer = typewriterPacer(ai, onUpdate)
  const { abort, done } = streamSSE(
    '/api/v1/ask/stream',
    { method: 'POST', body: { repo_id: repoId, question: q, mode, top_k: 8, history } },
    (frame) => {
      try {
        const payload = JSON.parse(frame.data)
        if (frame.event === 'references') {
          ai.references = payload.references
          onUpdate?.()
        } else if (frame.event === 'thinking') {
          // 推理段直接追加（折叠区，不走打字机）。
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
      // 显式停止：静默收尾，保留已生成内容。
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
    saveHistory(repoId, s.messages)
    onUpdate?.()
  }
}
