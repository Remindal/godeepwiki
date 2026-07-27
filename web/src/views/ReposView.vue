<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>仓库</h1>
        <p class="dw-muted">摄取代码仓库，开始问答与 Wiki 生成</p>
      </div>
      <el-button type="primary" @click="ingestVisible = true">＋ 摄取仓库</el-button>
    </header>

    <div v-if="loading" class="empty dw-faint">加载中…</div>
    <div v-else-if="!repos.length" class="empty">
      <div class="empty-icon">⌘</div>
      <p class="dw-muted">还没有仓库，点击右上角「摄取仓库」开始</p>
    </div>

    <div v-else class="repo-grid">
      <div v-for="r in repos" :key="r.repo_id" class="repo-card dw-card" @click="openRepo(r)">
        <div class="repo-top">
          <span class="repo-name">{{ repoName(r.repo_url) }}</span>
          <span class="state-pill" :class="r.state">{{ stateText(r.state) }}</span>
        </div>
        <div class="repo-url dw-faint">{{ r.repo_url }}@{{ r.branch }}</div>
        <div class="repo-stats dw-muted">
          <span>{{ r.stats?.chunks ?? r.chunk_count ?? 0 }} chunks</span>
          <span>{{ r.stats?.files ?? 0 }} files</span>
        </div>
        <div class="repo-actions" @click.stop>
          <el-button size="small" text @click.stop="$router.push(`/ask/${r.repo_id}`)" :disabled="r.state !== 'ready'">问答</el-button>
          <el-button size="small" text @click.stop="$router.push(`/wiki/${r.repo_id}`)" :disabled="r.state !== 'ready'">Wiki</el-button>
          <el-button size="small" text @click.stop="refresh(r)" :disabled="r.state !== 'ready'">刷新</el-button>
          <el-button size="small" text type="danger" @click.stop="remove(r)">删除</el-button>
        </div>
      </div>
    </div>

    <el-dialog v-model="ingestVisible" title="摄取仓库" width="440px">
      <el-form label-position="top">
        <el-form-item label="仓库地址">
          <el-input v-model="form.repo_url" placeholder="https://github.com/user/repo" />
        </el-form-item>
        <el-form-item label="分支（留空为默认分支）">
          <el-input v-model="form.branch" placeholder="master / main" />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="form.auto_wiki">摄取完成后自动生成 Wiki</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ingestVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitIngest">开始摄取</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, ApiError } from '../api/client'
import type { PageResult, Repo, TaskSubmitted } from '../api/types'

const router = useRouter()
const repos = ref<Repo[]>([])
const loading = ref(true)
const ingestVisible = ref(false)
const submitting = ref(false)
const form = ref({ repo_url: '', branch: '', auto_wiki: false })

const repoName = (url: string) => url.replace(/\.git$/, '').split('/').slice(-2).join('/')
const stateText = (s: string) => ({ ingesting: '摄取中', ready: '就绪', error: '失败' }[s] ?? s)

async function load() {
  loading.value = true
  try {
    const res = await api.get<PageResult<Repo>>('/api/v1/repos?page_size=100')
    repos.value = res.items ?? []
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

function openRepo(r: Repo) {
  if (r.state === 'ready') router.push(`/ask/${r.repo_id}`)
}

async function submitIngest() {
  if (!form.value.repo_url) {
    ElMessage.warning('请填写仓库地址')
    return
  }
  submitting.value = true
  try {
    const res = await api.post<TaskSubmitted>('/api/v1/ingest', form.value)
    ElMessage.success(`任务已提交：${res.task_id}`)
    ingestVisible.value = false
    form.value = { repo_url: '', branch: '', auto_wiki: false }
    router.push('/tasks')
  } catch (e) {
    if (e instanceof ApiError && e.code === 40901) {
      ElMessage.warning('该仓库已存在，请使用刷新')
    } else {
      ElMessage.error(e instanceof Error ? e.message : '提交失败')
    }
  } finally {
    submitting.value = false
  }
}

async function refresh(r: Repo) {
  try {
    const res = await api.post<TaskSubmitted>(`/api/v1/repos/${r.repo_id}/refresh`)
    ElMessage.success(`刷新任务已提交：${res.task_id}`)
    router.push('/tasks')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '刷新失败')
  }
}

async function remove(r: Repo) {
  try {
    await ElMessageBox.confirm(`删除 ${repoName(r.repo_url)}？将同时删除索引与本地目录。`, '确认删除', { type: 'warning' })
  } catch {
    return
  }
  try {
    await api.del(`/api/v1/repos/${r.repo_id}`)
    ElMessage.success('已删除')
    load()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '删除失败')
  }
}

onMounted(load)
</script>

<style scoped>
.page { max-width: 960px; margin: 0 auto; padding: 32px 24px; }
.page-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 24px; }
h1 { font-size: 22px; margin: 0 0 4px; font-weight: 600; }
.page-head p { margin: 0; font-size: 13px; }

.empty { text-align: center; padding: 80px 0; }
.empty-icon { font-size: 40px; color: var(--dw-text-3); margin-bottom: 12px; }

.repo-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 14px; }
.repo-card { padding: 16px; cursor: pointer; transition: box-shadow 0.15s ease; }
.repo-card:hover { box-shadow: var(--dw-shadow-hover); }
.repo-top { display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; }
.repo-name { font-weight: 600; font-size: 14px; }
.repo-url { font-size: 12px; margin-bottom: 10px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.repo-stats { display: flex; gap: 12px; font-size: 12px; margin-bottom: 8px; }
.repo-actions { border-top: 1px solid var(--dw-border); padding-top: 6px; margin: 0 -4px -6px; }

.state-pill {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--dw-bg-mute);
  color: var(--dw-text-2);
}
.state-pill.ready { background: #000; color: #fff; }
.state-pill.ingesting { background: var(--dw-bg-mute); color: var(--dw-text); }
.state-pill.error { background: #000; color: #fff; }
</style>
