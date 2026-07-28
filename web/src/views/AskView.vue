<template>
  <div class="ask-page">
    <div class="chat-area" ref="scrollEl">
      <div class="chat-inner">
        <div v-if="!messages.length" class="chat-empty">
          <div class="empty-logo">DW</div>
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
            <div class="ai-avatar">DW</div>
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

    <div v-if="messages.length" class="chat-toolbar">
      <button class="clear-btn dw-faint" @click="clearHistory">清空对话</button>
    </div>

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
          <el-select v-model="mode" size="small" class="mode-select" :disabled="streaming">
            <el-option label="混合检索" value="hybrid" />
            <el-option label="向量检索" value="embedding" />
            <el-option label="关键词检索" value="keyword" />
          </el-select>
          <button class="send-btn" :disabled="!input.trim() || streaming" @click="submit(input)">
            {{ streaming ? '…' : '↑' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { marked } from 'marked'
import { ask, clearChat, getChat, type ChatMsg } from '../stores/chat'

const route = useRoute()
const input = ref('')
const mode = ref('hybrid')
const scrollEl = ref<HTMLElement>()

// repoId 跟随路由参数响应式变化（/ask/A → /ask/B 组件复用时 setup 不重跑）。
const repoId = computed(() => route.params.repoId as string)
// 会话来自模块级仓库：离开页面流式在后台续跑，回来接着看（市面 AI 对话行为）。
const chat = computed(() => getChat(repoId.value))
const messages = computed(() => chat.value.messages)
const streaming = computed(() => chat.value.streaming)

const hints = ['这个仓库是干什么的？', '核心入口函数在哪里？', '路由是怎么注册和匹配的？']

function renderMd(text: string) {
  return marked.parse(text || '', { async: false })
}

// “已思考 N 秒”：1s 节拍器驱动重渲染（仅展示层计时，不影响后台流）。
const nowTs = ref(Date.now())
let ticker: number | undefined
function elapsed(m: ChatMsg): number {
  void nowTs.value // 依赖追踪：每秒触发一次重算
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
  if (!q || streaming.value) return
  input.value = ''
  // fire-and-forget：组件只负责发起与滚动，生命周期不影响后台续跑。
  void ask(repoId.value, q, mode.value, scrollBottom)
}

function clearHistory() {
  clearChat(repoId.value)
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
.send-btn {
  width: 32px; height: 32px; border: none; border-radius: 50%;
  background: var(--dw-black); color: var(--dw-white);
  font-size: 16px; cursor: pointer;
  display: flex; align-items: center; justify-content: center;
}
.send-btn:disabled { opacity: 0.3; cursor: not-allowed; }
</style>
