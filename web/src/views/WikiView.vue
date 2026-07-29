<template>
  <div class="wiki-page">
    <aside class="wiki-toc">
      <div class="toc-head">
        <span class="toc-title">Wiki</span>
        <div>
          <el-button v-if="wiki" size="small" text @click="download">下载</el-button>
          <el-button size="small" text @click="generate" :loading="generating">重新生成</el-button>
        </div>
      </div>
      <div v-if="wiki?.toc?.length" class="toc-list">
        <div
          v-for="item in wiki.toc"
          :key="item.slug"
          class="toc-item"
          :class="{ active: current?.slug === item.slug }"
          @click="current = wiki!.pages.find((p) => p.slug === item.slug) ?? null"
        >
          {{ item.title }}
        </div>
      </div>
      <div v-else class="dw-faint toc-empty">尚未生成</div>
    </aside>

    <main class="wiki-content">
      <div v-if="generating" class="center dw-muted">正在生成 Wiki，这可能需要十几分钟…</div>
      <div v-else-if="current" class="content-inner">
        <h1 class="page-title">{{ current.title }}</h1>
        <div ref="mdEl" class="md" v-html="renderMd(current.content_md)"></div>
      </div>
      <div v-else class="center">
        <p class="dw-muted">这个仓库还没有 Wiki</p>
        <el-button type="primary" @click="generate" :loading="generating">生成 Wiki</el-button>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { marked } from 'marked'
import mermaid from 'mermaid'
import { ElMessage } from 'element-plus'
import { api, ApiError } from '../api/client'
import type { TaskSubmitted, Wiki, WikiPage } from '../api/types'

mermaid.initialize({ startOnLoad: false, theme: 'neutral', securityLevel: 'strict' })
const mdEl = ref<HTMLElement>()

// mermaid 代码块渲染为 <pre class="mermaid">，随后交给 mermaid.run 出图。
marked.use({
  renderer: {
    code(code: string, infostring?: string) {
      if (infostring?.trim() === 'mermaid') {
        return `<pre class="mermaid">${escapeHtml(code)}</pre>`
      }
      return false as unknown as string
    },
  },
})

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

async function renderMermaid() {
  await nextTick()
  const nodes = mdEl.value?.querySelectorAll('pre.mermaid')
  if (nodes?.length) {
    try {
      await mermaid.run({ nodes: Array.from(nodes) as HTMLElement[] })
    } catch { /* 图渲染失败保留源码块 */ }
  }
}

const route = useRoute()
const repoId = route.params.repoId as string
const wiki = ref<Wiki | null>(null)
const current = ref<WikiPage | null>(null)
const generating = ref(false)

const renderMd = (md: string) => marked.parse(md || '', { async: false })

watch(current, () => {
  void renderMermaid()
})

async function load() {
  try {
    wiki.value = await api.get<Wiki>(`/api/v1/repos/${repoId}/wiki`)
    current.value = wiki.value.pages?.[0] ?? null
    void renderMermaid()
  } catch (e) {
    if (!(e instanceof ApiError && e.code === 40403)) {
      ElMessage.error(e instanceof Error ? e.message : '加载失败')
    }
  }
}

function download() {
  // 浏览器原生下载（响应为 attachment 字节流，不走 envelope）。
  window.open(`/api/v1/repos/${repoId}/wiki/export`, '_blank')
}

async function generate() {
  generating.value = true
  try {
    const res = await api.post<TaskSubmitted>('/api/v1/wiki/generate', { repo_id: repoId })
    ElMessage.success(`生成任务已提交：${res.task_id}，完成后回到本页查看`)
    setTimeout(load, 15000)
  } catch (e) {
    if (e instanceof ApiError && e.code === 40902) {
      ElMessage.warning(e.message) // "wiki 正在生成中，请等待完成"
    } else {
      ElMessage.error(e instanceof Error ? e.message : '提交失败')
    }
    generating.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.wiki-page { display: flex; height: 100%; }
.wiki-toc {
  width: 220px; flex-shrink: 0;
  border-right: 1px solid var(--dw-border);
  background: var(--dw-bg-soft);
  padding: 16px 12px;
  overflow-y: auto;
}
.toc-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
.toc-title { font-weight: 600; font-size: 14px; }
.toc-list { display: flex; flex-direction: column; gap: 2px; }
.toc-item {
  padding: 7px 10px; border-radius: var(--dw-radius-sm);
  font-size: 13px; color: var(--dw-text-2); cursor: pointer;
}
.toc-item:hover { background: var(--dw-bg-mute); }
.toc-item.active { background: var(--dw-white); color: var(--dw-text); font-weight: 500; box-shadow: var(--dw-shadow); }
.toc-empty { font-size: 12px; padding: 8px; }

.wiki-content { flex: 1; overflow-y: auto; }
.center { text-align: center; padding: 100px 20px; }
.content-inner { max-width: 760px; margin: 0 auto; padding: 32px 24px; }
.page-title { font-size: 24px; margin: 0 0 20px; }
.md { font-size: 14px; line-height: 1.8; }
.md :deep(pre) { background: var(--dw-bg-soft); border-radius: var(--dw-radius-sm); padding: 12px; overflow-x: auto; }
.md :deep(h1), .md :deep(h2), .md :deep(h3) { margin-top: 24px; }
</style>
