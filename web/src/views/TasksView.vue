<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>任务中心</h1>
        <p class="dw-muted">摄取 / 刷新 / Wiki 任务的实时进度</p>
      </div>
      <div class="head-side">
        <span v-if="lastRefresh" class="dw-faint refresh-time">更新于 {{ lastRefresh }}</span>
        <el-button @click="load(true)" :loading="loading">刷新</el-button>
      </div>
    </header>

    <div v-if="!tasks.length && !loading" class="empty dw-faint">暂无任务</div>

    <div v-else class="task-list">
      <div v-for="t in tasks" :key="t.task_id" class="task-card dw-card">
        <div class="task-top">
          <div class="task-meta">
            <span class="task-type">{{ typeText(t.type) }}</span>
            <span class="dw-faint task-id">{{ t.task_id }}</span>
          </div>
          <span class="state-text" :class="{ failed: t.state === 'failed' }">{{ t.state }}</span>
        </div>
        <el-progress
          :percentage="t.progress?.percent ?? 0"
          :stroke-width="6"
          :show-text="false"
          :color="t.state === 'failed' ? '#000' : '#1a1a1a'"
        />
        <div v-if="t.error" class="task-error">失败于 {{ t.error.stage }}：{{ t.error.message }}</div>
        <div class="task-bottom dw-faint">
          <span>{{ t.stats?.files ?? 0 }} 文件 · {{ t.stats?.chunks ?? 0 }} 块</span>
          <span>{{ formatTime(t.created_at) }}</span>
          <el-button
            v-if="!isTerminal(t.state)"
            size="small"
            text
            type="danger"
            @click="cancel(t)"
          >取消</el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../api/client'
import type { PageResult, Task } from '../api/types'

const tasks = ref<Task[]>([])
const loading = ref(false)
const lastRefresh = ref('')
let timer: number | undefined

const typeText = (t: string) => ({ ingest: '摄取', refresh: '刷新', wiki: 'Wiki' }[t] ?? t)
const isTerminal = (s: string) => ['completed', 'failed', 'cancelled'].includes(s)
const formatTime = (s: string) => new Date(s).toLocaleString()

async function load(manual = false) {
  loading.value = true
  try {
    const res = await api.get<PageResult<Task>>('/api/v1/tasks?page_size=50')
    tasks.value = res.items ?? []
    lastRefresh.value = new Date().toLocaleTimeString()
    if (manual) ElMessage.success('已刷新')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function cancel(t: Task) {
  try {
    await api.del(`/api/v1/tasks/${t.task_id}`)
    ElMessage.success('已取消')
    load()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '取消失败')
  }
}

// WebSocket 实时推送（替代轮询）：事件到达即刷新（去抖 500ms）；断线 3s 重连并带
// resume_from 回放漏掉的事件。WS 未就绪期间保留 3s 轮询兜底。
let ws: WebSocket | undefined
let lastSeq = 0
let wsOpen = false
let debounce: number | undefined

function debouncedLoad() {
  clearTimeout(debounce)
  debounce = window.setTimeout(() => load(), 500)
}

function connectWS() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const url = `${proto}://${location.host}/api/v1/ws?resume_from=${lastSeq}`
  ws = new WebSocket(url)
  ws.onopen = () => {
    wsOpen = true
  }
  ws.onmessage = (e) => {
    try {
      const frame = JSON.parse(e.data) as { seq: number; type: string }
      if (frame.seq) lastSeq = frame.seq
      debouncedLoad()
    } catch { /* 忽略非 JSON 帧 */ }
  }
  ws.onclose = () => {
    wsOpen = false
    window.setTimeout(connectWS, 3000)
  }
  ws.onerror = () => ws?.close()
}

onMounted(() => {
  load()
  connectWS()
  timer = window.setInterval(() => {
    // WS 断开期间的兜底轮询；WS 正常时事件驱动即可。
    if (!wsOpen && tasks.value.some((t) => !isTerminal(t.state))) load()
  }, 3000)
})
onBeforeUnmount(() => {
  clearInterval(timer)
  ws?.close()
})
</script>

<style scoped>
.page { max-width: 760px; margin: 0 auto; padding: 32px 24px; }
.page-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 24px; }
.head-side { display: flex; align-items: center; gap: 10px; }
.refresh-time { font-size: 12px; }
h1 { font-size: 22px; margin: 0 0 4px; font-weight: 600; }
.page-head p { margin: 0; font-size: 13px; }
.empty { text-align: center; padding: 80px 0; }

.task-list { display: flex; flex-direction: column; gap: 12px; }
.task-card { padding: 14px 16px; }
.task-top { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
.task-meta { display: flex; align-items: center; gap: 8px; }
.task-type { font-weight: 600; font-size: 13px; }
.task-id { font-size: 11px; font-family: ui-monospace, monospace; }
.state-text { font-size: 12px; color: var(--dw-text-2); }
.state-text.failed { color: #000; font-weight: 600; }
.task-error { font-size: 12px; color: var(--dw-text); background: var(--dw-bg-soft); border-radius: 8px; padding: 6px 10px; margin-top: 8px; }
.task-bottom { display: flex; align-items: center; gap: 14px; font-size: 12px; margin-top: 8px; }
</style>
