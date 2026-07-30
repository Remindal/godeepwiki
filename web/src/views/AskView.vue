<template>
  <div class="ask-page">
    <div class="conv-bar">
      <el-select :model-value="convId" size="small" class="conv-select" @change="switchConv">
        <el-option v-for="c in convs" :key="c.id" :label="c.title" :value="c.id" />
      </el-select>
      <button class="conv-btn" title="新建对话" @click="newConv">＋</button>
      <button class="conv-btn danger" title="删除当前对话" @click="removeConv(convId)">🗑</button>
    </div>

    <div class="chat-area" ref="scrollEl">
      <div class="chat-inner">
        <div v-if="!messages.length" class="chat-empty">
          <div class="empty-logo">GW</div>
          <p class="dw-muted">向这个仓库提问，回答都会附上代码出处</p>
          <div class="hints">
            <div v-for="h in hints" :key="h" class="hint dw-card" @click="submit(h)">{{ h }}</div>
          </div>
        </div>

        <template v-for="(m, i) in messages" :key="i">
          <!-- 用户气泡 -->
          <div v-if="m.role === 'user'" class="msg-row user">
            <div class="bubble user-bubble">{{ m.content }}</div>
          </div>

          <!-- AI 气泡 -->
          <div v-else class="msg-row">
            <div class="ai-avatar">GW</div>
            <div class="ai-body">
              <div v-if="m.thinking" class="thinking-wrap">
                <div class="thinking-title dw-faint" @click="m.thinkingOpen = !m.thinkingOpen">
                  {{ m.thinkingOpen ? '▾' : '▸' }} 思考过程{{ m.streaming && !m.content ? '（进行中…）' : '' }}
                </div>
                <div v-show="m.thinkingOpen" class="thinking-content dw-faint">{{ m.thinking }}</div>
              </div>
              <div class="bubble ai-bubble" v-html="renderMd(m.content)"></div>
              <div v-if="m.streaming && !m.content" class="typing dw-faint">
                思考中<span v-if="m.startedAt"> · 已思考 {{ elapsed(m) }} 秒</span>…
              </div>

              <div v-if="m.references?.length" class="refs">
                <div class="refs-title dw-faint" @click="m.refsOpen = !m.refsOpen">
                  {{ m.refsOpen ? '▾' : '▸' }} 引用来源（{{ m.references.length }}）
                </div>
                <div v-show="m.refsOpen" class="refs-list">
                  <div v-for="r in m.references" :key="r.chunk_id" class="ref-item">
                    <div class="ref-path clickable" title="点击查看完整代码" @click="openRef(r)">
                      [{{ r.path }}:{{ r.start_line }}-{{ r.end_line }}]
                    </div>
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

    <div v-if="messages.length" class="chat-toolbar">
      <button class="clear-btn dw-faint" @click="clearHistory">清空对话</button>
    </div>

    <el-drawer v-model="drawerOpen" :title="drawerChunk ? `${drawerChunk.path}:${drawerChunk.start_line}-${drawerChunk.end_line}` : '代码'" size="60%">
      <div v-if="drawerLoading" class="dw-faint">加载中…</div>
      <div v-else-if="drawerChunk" class="code-view">
        <div v-for="l in drawerLines" :key="l.no" class="code-line">
          <span class="line-no">{{ l.no }}</span>
          <span class="line-text">{{ l.text || ' ' }}</span>
        </div>
      </div>
    </el-drawer>

    <div class="input-bar">
      <div class="input-inner dw-card">
        <textarea
          v-model="input"
          class="chat-input"
          rows="1"
          placeholder="提问，Enter 发送，Shift+Enter 换行"
          :disabled="streaming"
          @keydown.enter.exact.prevent="submit(input)"
        />
        <div class="input-side">
          <span class="pf-wrap">
            <span v-if="pathTip" class="pf-tip">{{ pathTip }}</span>
            <input
              v-model="pathFilter"
              class="path-filter-input"
              :class="{ invalid: !!pathTip }"
              placeholder="限定目录（可留空）"
              title="只在此路径前缀内检索，如 app/Services/ 或 routes/web.php"
              :disabled="streaming"
            />
          </span>
          <el-select v-model="mode" size="small" class="mode-select" :disabled="streaming">
            <el-option label="混合检索" value="hybrid" />
            <el-option label="向量检索" value="embedding" />
            <el-option label="关键词检索" value="keyword" />
          </el-select>
          <button v-if="streaming" class="send-btn stop" title="停止生成" @click="stop">■</button>
          <button v-else class="send-btn" :disabled="!input.trim()" @click="submit(input)">↑</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { marked } from 'marked'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ask, clearChat, createConversation, deleteConversation, getChat,
  listConversations, stopChat, type ChatMsg,
} from '../stores/chat'
import { api } from '../api/client'
import type { Reference } from '../api/types'

const route = useRoute()
const router = useRouter()
const input = ref('')
const mode = ref('hybrid')
const pathFilter = ref(localStorage.getItem(`dw_pathfilter_${route.params.repoId}`) || '')
const scrollEl = ref<HTMLElement>()

// repoId 跟随路由参数响应式变化；convId 跟随 query.c（默认 default）。
const repoId = computed(() => route.params.repoId as string)
const convId = computed(() => (route.query.c as string) || 'default')
const chat = computed(() => getChat(repoId.value, convId.value))
const messages = computed(() => chat.value.messages)
const streaming = computed(() => chat.value.streaming)

// path_filter 校验：先本地格式校验，合法则去抖 500ms 调后端做仓库内存在性校验。
const pathFilterInvalid = computed(() => {
  const p = pathFilter.value
  if (!p) return ''
  if (p.includes('..')) return "不能包含 '..'"
  if (p.includes('\\')) return '请用 / 作为路径分隔符'
  if (p.startsWith('/')) return '请填仓库内相对路径，不要以 / 开头'
  if (p.length > 256) return '过长（≤256）'
  return ''
})

const pathStatus = ref<'ok' | 'checking' | 'missing'>('ok')
let pathCheckTimer: number | undefined
watch([pathFilter, repoId], () => {
  pathStatus.value = 'ok'
  clearTimeout(pathCheckTimer)
  const p = pathFilter.value.trim()
  if (!p || pathFilterInvalid.value) return
  pathCheckTimer = window.setTimeout(async () => {
    pathStatus.value = 'checking'
    try {
      const res = await api.get<{ exists: boolean }>(
        `/api/v1/repos/${repoId.value}/paths/exists?prefix=${encodeURIComponent(p)}`,
      )
      // 期间输入又变了则丢弃本次结果
      if (pathFilter.value.trim() === p) {
        pathStatus.value = res.exists ? 'ok' : 'missing'
      }
    } catch {
      pathStatus.value = 'ok' // 校验接口失败不阻塞输入
    }
  }, 500)
})

const pathTip = computed(() => {
  if (pathFilterInvalid.value) return pathFilterInvalid.value
  if (pathFilter.value.trim() && pathStatus.value === 'checking') return '校验中…'
  if (pathFilter.value.trim() && pathStatus.value === 'missing') return '仓库中不存在该路径'
  return ''
})

// 会话列表（当前仓库）。
const convs = ref(listConversations(repoId.value))
watch(repoId, () => {
  convs.value = listConversations(repoId.value)
})
watch(convId, () => {
  convs.value = listConversations(repoId.value)
})

function switchConv(id: string) {
  router.push({ query: id === 'default' ? {} : { c: id } })
}
function newConv() {
  const meta = createConversation(repoId.value)
  convs.value = listConversations(repoId.value)
  switchConv(meta.id)
}
async function removeConv(id: string) {
  try {
    await ElMessageBox.confirm('删除这个对话及其历史记录？', '确认删除', { type: 'warning' })
  } catch {
    return
  }
  deleteConversation(repoId.value, id)
  convs.value = listConversations(repoId.value)
  if (convId.value === id) switchConv(convs.value[0]?.id ?? 'default')
}

const hints = ['这个仓库是干什么的？', '核心入口函数在哪里？', '路由是怎么注册和匹配的？']

function renderMd(text: string) {
  return marked.parse(text || '', { async: false })
}

// “已思考 N 秒”：1s 节拍器驱动重渲染（仅展示层计时，不影响后台流）。
const nowTs = ref(Date.now())
let ticker: number | undefined
function elapsed(m: ChatMsg): number {
  void nowTs.value
  return m.startedAt ? Math.max(0, Math.floor((Date.now() - m.startedAt) / 1000)) : 0
}
onMounted(() => {
  ticker = window.setInterval(() => {
    if (streaming.value) nowTs.value = Date.now()
  }, 1000)
})
onBeforeUnmount(() => clearInterval(ticker))

async function scrollBottom() {
  await nextTick()
  scrollEl.value?.scrollTo({ top: scrollEl.value.scrollHeight })
}

function submit(question: string) {
  const q = question.trim()
  if (!q || streaming.value || pathFilterInvalid.value || pathStatus.value === 'missing') {
    if (pathTip.value) ElMessage.warning(`目录：${pathTip.value}`)
    return
  }
  input.value = ''
  localStorage.setItem(`dw_pathfilter_${repoId.value}`, pathFilter.value)
  void ask(repoId.value, convId.value, q, mode.value, pathFilter.value, scrollBottom)
}

function stop() {
  stopChat(repoId.value, convId.value)
}

function clearHistory() {
  clearChat(repoId.value, convId.value)
}

// 引用代码查看器（drawer）：点引用按 chunk_id 取全文带行号展示。
const drawerOpen = ref(false)
const drawerLoading = ref(false)
const drawerChunk = ref<{ path: string; start_line: number; end_line: number; language: string; content: string } | null>(null)

const drawerLines = computed(() => {
  if (!drawerChunk.value) return []
  return drawerChunk.value.content.split('\n').map((text, i) => ({
    no: drawerChunk.value!.start_line + i,
    text,
  }))
})

async function openRef(r: Reference) {
  drawerOpen.value = true
  drawerLoading.value = true
  drawerChunk.value = null
  try {
    drawerChunk.value = await api.get(`/api/v1/chunks/${r.chunk_id}`)
  } catch {
    ElMessage.error('加载代码块失败')
    drawerOpen.value = false
  } finally {
    drawerLoading.value = false
  }
}
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
.thinking-wrap { margin-bottom: 6px; }
.thinking-title { font-size: 12px; cursor: pointer; user-select: none; }
.thinking-content {
  font-size: 12px;
  line-height: 1.7;
  margin-top: 4px;
  padding: 8px 10px;
  background: var(--dw-bg-soft);
  border-left: 2px solid var(--dw-border);
  border-radius: 0 var(--dw-radius-sm) var(--dw-radius-sm) 0;
  white-space: pre-wrap;
  max-height: 240px;
  overflow-y: auto;
}
.typing { font-size: 13px; }

.refs { margin-top: 8px; }
.refs-title { font-size: 12px; cursor: pointer; user-select: none; }
.refs-list { margin-top: 6px; display: flex; flex-direction: column; gap: 6px; }
.ref-item { background: var(--dw-bg-soft); border-radius: var(--dw-radius-sm); padding: 8px 10px; }
.ref-path { font-size: 12px; font-weight: 600; font-family: ui-monospace, monospace; }
.ref-snippet { font-size: 12px; margin-top: 2px; }
.usage { font-size: 11px; margin-top: 6px; }

.chat-toolbar {
  max-width: 760px;
  margin: 0 auto;
  padding: 0 20px 4px;
  display: flex;
  justify-content: flex-end;
}
.clear-btn {
  border: none;
  background: none;
  font-size: 12px;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 6px;
}
.clear-btn:hover { background: var(--dw-bg-mute); color: var(--dw-text); }

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
.path-filter-input {
  width: 150px;
  border: none;
  outline: none;
  font: inherit;
  font-size: 12px;
  color: var(--dw-text-2);
  background: var(--dw-bg-soft);
  border-radius: 8px;
  padding: 5px 8px;
}
.path-filter-input::placeholder { color: var(--dw-text-3); }
.path-filter-input.invalid {
  background: #f5f5f5;
  outline: 1px solid #d93026;
}

.pf-wrap { position: relative; display: inline-flex; }
.pf-tip {
  position: absolute;
  bottom: calc(100% + 6px);
  left: 0;
  background: #d93026;
  color: #fff;
  font-size: 11px;
  line-height: 1.4;
  padding: 3px 9px;
  border-radius: 999px; /* 红色圆角胶囊提示 */
  white-space: nowrap;
  pointer-events: none;
  z-index: 10;
}
.pf-tip::after {
  content: '';
  position: absolute;
  top: 100%;
  left: 14px;
  border: 4px solid transparent;
  border-top-color: #d93026;
}
.send-btn {
  width: 32px; height: 32px; border: none; border-radius: 50%;
  background: var(--dw-black); color: var(--dw-white);
  font-size: 16px; cursor: pointer;
  display: flex; align-items: center; justify-content: center;
}
.send-btn:disabled { opacity: 0.3; cursor: not-allowed; }
.send-btn.stop { background: var(--dw-black); font-size: 12px; }

.conv-bar {
  max-width: 760px;
  margin: 0 auto;
  padding: 14px 20px 0;
  display: flex;
  align-items: center;
  gap: 6px;
}
.conv-select { width: 220px; }
.conv-btn {
  width: 28px; height: 28px;
  border: 1px solid var(--dw-border); border-radius: 8px;
  background: var(--dw-white); cursor: pointer; font-size: 13px;
  display: flex; align-items: center; justify-content: center;
}
.conv-btn:hover { box-shadow: var(--dw-shadow); }
.conv-btn.danger:hover { color: #000; }

.ref-path.clickable { cursor: pointer; }
.ref-path.clickable:hover { text-decoration: underline; }

.code-view { font-family: ui-monospace, 'SF Mono', Consolas, monospace; font-size: 12.5px; line-height: 1.6; }
.code-line { display: flex; white-space: pre; }
.line-no {
  flex-shrink: 0;
  width: 48px;
  text-align: right;
  padding-right: 12px;
  color: var(--dw-text-3);
  user-select: none;
}
.line-text { flex: 1; }
</style>
