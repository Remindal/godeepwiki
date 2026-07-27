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
          <span class="dot" :class="{ ok: item.ok }" />
          {{ item.ok ? '正常' : '异常' }}
        </div>
        <div class="dep-detail dw-faint">{{ item.detail }}</div>
      </div>

      <div class="dep-card dw-card">
        <div class="dep-name">Worker</div>
        <div class="dep-state"><span class="dot ok" />运行中</div>
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

const health = ref<Health | null>(null)
const loading = ref(false)

const items = computed(() => {
  if (!health.value) return []
  const h = health.value
  return [
    { name: 'LLM', ok: h.llm.reachable, detail: `${h.llm.provider}/${h.llm.model} · breaker=${h.llm.breaker}` },
    { name: 'Embedding', ok: h.embedding.reachable, detail: `${h.embedding.provider}/${h.embedding.model}` },
    { name: 'Postgres', ok: h.postgres.connected, detail: 'pgvector' },
    { name: 'OpenSearch', ok: h.opensearch.connected, detail: 'BM25 索引' },
    { name: 'RabbitMQ', ok: h.rabbitmq.connected, detail: `queue_depth=${h.rabbitmq.queue_depth}` },
    { name: 'Redis', ok: h.redis.connected, detail: h.redis.ratelimit_degraded ? '限流降级中' : 'sentinel' },
    { name: 'etcd', ok: h.etcd.connected, detail: '配置中心' },
    { name: 'Git', ok: h.git.available, detail: 'git CLI' },
  ]
})

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
.dot { width: 7px; height: 7px; border-radius: 50%; background: var(--dw-text-3); }
.dot.ok { background: var(--dw-black); }
.dep-detail { font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
