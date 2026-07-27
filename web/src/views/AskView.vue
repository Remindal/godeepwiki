<template>
  <div class="ask-page">
    <div class="chat-area" ref="scrollEl">
      <div class="chat-inner">
        <div v-if="!messages.length" class="chat-empty">
          <div class="empty-logo">DW</div>
          <p class="dw-muted">向这个仓库提问，回答都会附上代码出处</p>
          <div class="hints">
            <div v-for="h in hints" :key="h" class="hint dw-card" @click="ask(h)">{{ h }}</div>
          </div>
        </div>

        <template v-for="(m, i) in messages" :key="i">
          <!-- 用户气泡 -->
          <div v-if="m.role === 'user'" class="msg-row user">
            <div class="bubble user-bubble">{{ m.content }}</div>
          </div>

          <!-- AI 气泡 -->
          <div v-else class="msg-row">
            <div class="ai-avatar">DW</div>
            <div class="ai-body">
              <div v-if="m.thinking" class="thinking dw-faint">{{ m.thinking }}</div>
              <div class="bubble ai-bubble" v-html="renderMd(m.content)"></div>
              <div v-if="m.streaming && !m.content" class="typing dw-faint">思考中…</div>

              <div v-if="m.references?.length" class="refs">
                <div class="refs-title dw-faint" @click="m.refsOpen = !m.refsOpen">
                  {{ m.refsOpen ? '▾' : '▸' }} 引用来源（{{ m.references.length }}）
                </div>
                <div v-show="m.refsOpen" class="refs-list">
                  <div v-for="r in m.references" :key="r.chunk_id" class="ref-item">
                    <div class="ref-path">[{{ r.path }}:{{ r.start_line }}-{{ r.end_line }}]</div>
                    <div class="ref-snippet dw-faint">{{ r.snippet.slice(0, 160) }}…</div>
                  </div>
                </div>
              </div>

              <div v-if="m.usage" class="usage dw-faint">
                {{ m.usage.prompt_tokens }}+{{ m.usage.completion_tokens }} tokens · {{ m.latency_ms }}ms
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>

    <div class="input-bar">
      <div class="input-inner dw-card">
        <textarea
          v-model="input"
          class="chat-input"
          rows="1"
          placeholder="提问，Enter 发送，Shift+Enter 换行"
          :disabled="streaming"
          @keydown.enter.exact.prevent="ask(input)"
        />
        <div class="input-side">
          <el-select v-model="mode" size="small" class="mode-select" :disabled="streaming">
            <el-option label="混合检索" value="hybrid" />
            <el-option label="向量检索" value="embedding" />
            <el-option label="关键词检索" value="keyword" />
          </el-select>
          <button class="send-btn" :disabled="!input.trim() || streaming" @click="ask(input)">
            {{ streaming ? '…' : '↑' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref } from 'vue'
import { useRoute } from 'vue-router'
import { marked } from 'marked'
import { ElMessage } from 'element-plus'
import { streamSSE, ApiError } from '../api/client'
import type { Reference } from '../api/types'

interface Msg {
  role: 'user' | 'ai'
  content: string
  thinking?: string
  streaming?: boolean
  references?: Reference[]
  refsOpen?: boolean
  usage?: { prompt_tokens: number; completion_tokens: number }
  latency_ms?: number
}

const route = useRoute()
const repoId = route.params.repoId as string
const input = ref('')
const mode = ref('hybrid')
const streaming = ref(false)
const messages = ref<Msg[]>([])
const scrollEl = ref<HTMLElement>()

const hints = ['这个仓库是干什么的？', '核心入口函数在哪里？', '路由是怎么注册和匹配的？']

let abort: (() => void) | null = null

function renderMd(text: string) {
  return marked.parse(text || '', { async: false })
}

async function scrollBottom() {
  await nextTick()
  scrollEl.value?.scrollTo({ top: scrollEl.value.scrollHeight })
}

async function ask(question: string) {
  const q = question.trim()
  if (!q || streaming.value) return
  input.value = ''
  messages.value.push({ role: 'user', content: q })
  const ai: Msg = { role: 'ai', content: '', thinking: '', streaming: true, refsOpen: false }
  messages.value.push(ai)
  streaming.value = true
  scrollBottom()

  const { abort: ab, done } = streamSSE(
    '/api/v1/ask/stream',
    { method: 'POST', body: { repo_id: repoId, question: q, mode: mode.value, top_k: 8 } },
    (frame) => {
      try {
        const payload = JSON.parse(frame.data)
        if (frame.event === 'references') {
          ai.references = payload.references
        } else if (frame.event === 'token') {
          // thinking 模型的推理与正文统一流式展示；推理段落弱化显示
          ai.content += payload.delta
        } else if (frame.event === 'done') {
          ai.usage = payload.usage
          ai.latency_ms = payload.latency_ms
          ai.streaming = false
        } else if (frame.event === 'error') {
          ai.content = `出错了：${payload.message}`
          ai.streaming = false
        }
        scrollBottom()
      } catch { /* 忽略非 JSON 帧 */ }
    },
  )
  abort = ab
  try {
    await done
  } catch (e) {
    ai.streaming = false
    if (e instanceof ApiError) ai.content = `出错了：${e.message}`
    else if ((e as Error).name !== 'AbortError') ai.content = '连接中断，请重试'
    ElMessage.error(ai.content)
  } finally {
    streaming.value = false
    abort = null
    scrollBottom()
  }
}

onBeforeUnmount(() => abort?.())
</script>

<style scoped>
.ask-page { display: flex; flex-direction: column; height: 100%; }
.chat-area { flex: 1; overflow-y: auto; }
.chat-inner { max-width: 760px; margin: 0 auto; padding: 24px 20px 12px; }

.chat-empty { text-align: center; padding: 80px 0 40px; }
.empty-logo {
  width: 44px; height: 44px; margin: 0 auto 14px;
  border-radius: 13px; background: var(--dw-black); color: var(--dw-white);
  display: flex; align-items: center; justify-content: center; font-weight: 700;
}
.hints { display: flex; flex-direction: column; gap: 8px; max-width: 320px; margin: 20px auto 0; }
.hint { padding: 10px 14px; font-size: 13px; color: var(--dw-text-2); cursor: pointer; text-align: left; }
.hint:hover { box-shadow: var(--dw-shadow-hover); }

.msg-row { display: flex; gap: 10px; margin-bottom: 18px; }
.msg-row.user { justify-content: flex-end; }

.bubble {
  max-width: 78%;
  padding: 10px 14px;
  border-radius: var(--dw-radius-lg);
  font-size: 14px;
  line-height: 1.7;
  word-break: break-word;
}
.user-bubble { background: var(--dw-black); color: var(--dw-white); border-bottom-right-radius: 6px; }
.ai-bubble { background: transparent; padding: 2px 0; max-width: 100%; }
.ai-bubble :deep(pre) { background: var(--dw-bg-soft); border-radius: var(--dw-radius-sm); padding: 10px; overflow-x: auto; }
.ai-bubble :deep(code) { font-size: 13px; }

.ai-avatar {
  width: 28px; height: 28px; flex-shrink: 0; margin-top: 2px;
  border-radius: 8px; background: var(--dw-black); color: var(--dw-white);
  font-size: 11px; font-weight: 700; display: flex; align-items: center; justify-content: center;
}
.ai-body { flex: 1; min-width: 0; }
.thinking { font-size: 12px; margin-bottom: 4px; }
.typing { font-size: 13px; }

.refs { margin-top: 8px; }
.refs-title { font-size: 12px; cursor: pointer; user-select: none; }
.refs-list { margin-top: 6px; display: flex; flex-direction: column; gap: 6px; }
.ref-item { background: var(--dw-bg-soft); border-radius: var(--dw-radius-sm); padding: 8px 10px; }
.ref-path { font-size: 12px; font-weight: 600; font-family: ui-monospace, monospace; }
.ref-snippet { font-size: 12px; margin-top: 2px; }
.usage { font-size: 11px; margin-top: 6px; }

.input-bar { padding: 12px 20px 20px; }
.input-inner {
  max-width: 760px; margin: 0 auto;
  display: flex; align-items: flex-end; gap: 10px;
  padding: 10px 12px;
}
.chat-input {
  flex: 1; border: none; outline: none; resize: none;
  font: inherit; font-size: 14px; line-height: 1.6;
  max-height: 140px; background: transparent;
}
.input-side { display: flex; align-items: center; gap: 8px; }
.mode-select { width: 104px; }
.send-btn {
  width: 32px; height: 32px; border: none; border-radius: 50%;
  background: var(--dw-black); color: var(--dw-white);
  font-size: 16px; cursor: pointer;
  display: flex; align-items: center; justify-content: center;
}
.send-btn:disabled { opacity: 0.3; cursor: not-allowed; }
</style>
