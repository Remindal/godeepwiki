<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>系统状态</h1>
        <p class="dw-muted">依赖健康与 Worker 实时状态（60s 探测缓存）</p>
      </div>
      <el-button @click="load" :loading="loading">刷新</el-button>
    </header>

    <div v-if="health" class="grid">
      <div v-for="item in items" :key="item.name" class="dep-card dw-card">
        <div class="dep-name">{{ item.name }}</div>
        <div class="dep-state">
          <span class="dot" :class="item.level" />
          <span :class="'state-' + item.level">{{ levelText(item.level) }}</span>
        </div>
        <div class="dep-detail dw-faint">{{ item.detail }}</div>
      </div>

      <div class="dep-card dw-card">
        <div class="dep-name">Worker</div>
        <div class="dep-state">
          <span class="dot" :class="workerLevel" />
          <span :class="'state-' + workerLevel">{{ levelText(workerLevel) }}</span>
        </div>
        <div class="dep-detail dw-faint">
          busy {{ health.worker.busy }} / total {{ health.worker.total }} · queued {{ health.worker.queued }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '../api/client'
import type { Health } from '../api/types'

// 红黄绿三档：ok=正常(绿) / warn=降级(黄) / bad=异常(红)
type Level = 'ok' | 'warn' | 'bad'

const health = ref<Health | null>(null)
const loading = ref(false)

function providerLevel(reachable: boolean, breaker: string): Level {
  if (!reachable) return 'bad'
  return breaker === 'closed' ? 'ok' : 'warn'
}

const items = computed(() => {
  if (!health.value) return []
  const h = health.value
  return [
    { name: 'LLM', level: providerLevel(h.llm.reachable, h.llm.breaker), detail: `${h.llm.provider}/${h.llm.model} · breaker=${h.llm.breaker}` },
    { name: 'Embedding', level: providerLevel(h.embedding.reachable, h.embedding.breaker), detail: `${h.embedding.provider}/${h.embedding.model}` },
    { name: 'Postgres', level: (h.postgres.connected ? 'ok' : 'bad') as Level, detail: 'pgvector' },
    {
      name: 'OpenSearch',
      level: (!h.opensearch.connected ? 'bad'
        : h.opensearch.cluster_status === 'red' ? 'bad'
        : h.opensearch.cluster_status === 'yellow' ? 'warn' : 'ok') as Level,
      detail: `BM25 索引 · ${h.opensearch.cluster_status ?? ''}`,
    },
    {
      name: 'RabbitMQ',
      level: (!h.rabbitmq.connected ? 'bad' : h.rabbitmq.consumers === 0 ? 'warn' : 'ok') as Level,
      detail: `queue_depth=${h.rabbitmq.queue_depth} · consumers=${h.rabbitmq.consumers ?? '-'}`,
    },
    { name: 'Redis', level: (!h.redis.connected ? 'bad' : h.redis.ratelimit_degraded ? 'warn' : 'ok') as Level, detail: h.redis.ratelimit_degraded ? '限流降级中' : 'sentinel' },
    { name: 'etcd', level: (h.etcd.connected ? 'ok' : 'bad') as Level, detail: '配置中心' },
    { name: 'Git', level: (h.git.available ? 'ok' : 'bad') as Level, detail: 'git CLI' },
  ]
})

const workerLevel = computed<Level>(() => {
  if (!health.value) return 'ok'
  return health.value.worker.queued > 0 ? 'warn' : 'ok'
})

function levelText(l: Level): string {
  return l === 'ok' ? '正常' : l === 'warn' ? '降级' : '异常'
}

async function load() {
  loading.value = true
  try {
    health.value = await api.get<Health>('/api/v1/health')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.page { max-width: 760px; margin: 0 auto; padding: 32px 24px; }
.page-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 24px; }
h1 { font-size: 22px; margin: 0 0 4px; font-weight: 600; }
.page-head p { margin: 0; font-size: 13px; }

.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 12px; }
.dep-card { padding: 14px 16px; }
.dep-name { font-weight: 600; font-size: 13px; margin-bottom: 6px; }
.dep-state { display: flex; align-items: center; gap: 6px; font-size: 13px; margin-bottom: 4px; }

.dot { width: 9px; height: 9px; border-radius: 50%; flex-shrink: 0; }
.dot.ok { background: #22c55e; box-shadow: 0 0 0 3px rgba(34, 197, 94, 0.15); }
.dot.warn { background: #f59e0b; box-shadow: 0 0 0 3px rgba(245, 158, 11, 0.15); }
.dot.bad { background: #ef4444; box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.15); }

.state-ok { color: #16a34a; font-weight: 500; }
.state-warn { color: #d97706; font-weight: 500; }
.state-bad { color: #dc2626; font-weight: 500; }

.dep-detail { font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
