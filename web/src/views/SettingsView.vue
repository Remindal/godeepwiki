<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>设置</h1>
        <p class="dw-muted">模型与检索配置（PUT /api/v1/config 热更新，版本 {{ version }}）</p>
      </div>
    </header>

    <div v-if="loading" class="empty dw-faint">加载中…</div>

    <template v-else>
      <!-- LLM -->
      <section class="dw-card section">
        <h2>对话模型（LLM）</h2>
        <div class="form-grid">
          <label>Provider</label>
          <el-select v-model="form.llm.provider">
            <el-option v-for="p in llmProviders" :key="p" :label="p" :value="p" />
          </el-select>
          <label>模型</label>
          <el-input v-model="form.llm.model" placeholder="如 deepseek-ai/DeepSeek-V4-Flash" />
          <label>Base URL</label>
          <el-input v-model="form.llm.base_url" placeholder="如 https://api.siliconflow.cn/v1" />
          <label>Temperature</label>
          <el-input-number v-model="form.llm.temperature" :min="0" :max="2" :step="0.1" />
          <label>Max Tokens</label>
          <el-input-number v-model="form.llm.max_tokens" :min="256" :max="32768" :step="256" />
        </div>
        <div class="key-status dw-faint">
          API Key：{{ masked(form.llm.api_key) }}（密钥只能经部署环境变量 DEEPWIKI_LLM_API_KEY 注入，界面不可改）
        </div>
        <el-button type="primary" :loading="saving === 'llm'" @click="save('llm')">保存</el-button>
      </section>

      <!-- Embedding -->
      <section class="dw-card section">
        <h2>向量模型（Embedding）</h2>
        <div class="form-grid">
          <label>Provider</label>
          <el-select v-model="form.embedding.provider">
            <el-option v-for="p in embedProviders" :key="p" :label="p" :value="p" />
          </el-select>
          <label>模型</label>
          <el-input v-model="form.embedding.model" placeholder="如 BAAI/bge-large-zh-v1.5" />
          <label>Base URL</label>
          <el-input v-model="form.embedding.base_url" placeholder="留空用 provider 默认" />
          <label>Batch Size</label>
          <el-input-number v-model="form.embedding.batch_size" :min="1" :max="256" />
        </div>
        <div class="key-status dw-faint">
          API Key：{{ masked(form.embedding.api_key) }}（DEEPWIKI_EMBEDDING_API_KEY 注入）
        </div>
        <div class="dw-faint dim-note">
          注意：换 embedding 模型会触发维度探测；与库中向量维度不一致会被拒绝（需重建索引）。
        </div>
        <el-button type="primary" :loading="saving === 'embedding'" @click="save('embedding')">保存</el-button>
      </section>

      <!-- Retriever -->
      <section class="dw-card section">
        <h2>检索</h2>
        <div class="form-grid">
          <label>默认模式</label>
          <el-select v-model="form.retriever.mode">
            <el-option label="hybrid 混合" value="hybrid" />
            <el-option label="embedding 向量" value="embedding" />
            <el-option label="keyword 关键词" value="keyword" />
          </el-select>
          <label>Top K</label>
          <el-input-number v-model="form.retriever.top_k" :min="1" :max="30" />
        </div>
        <el-button type="primary" :loading="saving === 'retriever'" @click="save('retriever')">保存</el-button>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, ApiError } from '../api/client'

interface Cfg {
  llm: { provider: string; model: string; api_key: string; base_url: string; temperature: number; max_tokens: number }
  embedding: { provider: string; model: string; api_key: string; base_url: string; batch_size: number }
  retriever: { mode: string; top_k: number }
}

const loading = ref(true)
const saving = ref('')
const version = ref(0)
const form = ref<Cfg>({
  llm: { provider: 'openai', model: '', api_key: '', base_url: '', temperature: 0.2, max_tokens: 2048 },
  embedding: { provider: 'siliconflow', model: '', api_key: '', base_url: '', batch_size: 64 },
  retriever: { mode: 'hybrid', top_k: 8 },
})

const llmProviders = ['openai', 'gemini', 'claude', 'ollama', 'deepseek']
const embedProviders = ['openai', 'dashscope', 'siliconflow', 'ollama', 'voyage']

const masked = (k: string) => (k ? k : '未配置')

async function load() {
  loading.value = true
  try {
    const cfg = await api.get<Cfg & { version?: number }>('/api/v1/config')
    form.value.llm = { ...form.value.llm, ...cfg.llm }
    form.value.embedding = { ...form.value.embedding, ...cfg.embedding }
    form.value.retriever = { ...form.value.retriever, ...cfg.retriever }
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载配置失败')
  } finally {
    loading.value = false
  }
}

async function save(section: 'llm' | 'embedding' | 'retriever') {
  saving.value = section
  try {
    const patch: Record<string, unknown> = {}
    if (section === 'llm') {
      const l = form.value.llm
      patch.llm = { provider: l.provider, model: l.model, base_url: l.base_url, temperature: l.temperature, max_tokens: l.max_tokens }
    } else if (section === 'embedding') {
      const e = form.value.embedding
      patch.embedding = { provider: e.provider, model: e.model, base_url: e.base_url, batch_size: e.batch_size }
    } else {
      patch.retriever = { mode: form.value.retriever.mode, top_k: form.value.retriever.top_k }
    }
    const res = await api.put<{ version: number; restart_required: string[]; warnings: string[] }>('/api/v1/config', patch)
    version.value = res.version
    if (section !== 'retriever') {
      ElMessageBox.alert(
        '模型类配置需重启后端生效：docker compose restart app',
        '已保存',
        { type: 'warning' },
      )
    } else {
      ElMessage.success('已保存（热生效）')
    }
  } catch (e) {
    if (e instanceof ApiError) {
      ElMessage.error(`保存失败：${e.message}`)
    } else {
      ElMessage.error(e instanceof Error ? e.message : '保存失败')
    }
  } finally {
    saving.value = ''
  }
}

onMounted(load)
</script>

<style scoped>
.page { max-width: 720px; margin: 0 auto; padding: 32px 24px; }
.page-head { margin-bottom: 24px; }
h1 { font-size: 22px; margin: 0 0 4px; font-weight: 600; }
.page-head p { margin: 0; font-size: 13px; }
.empty { text-align: center; padding: 60px 0; }

.section { padding: 18px 20px; margin-bottom: 16px; }
.section h2 { font-size: 15px; margin: 0 0 14px; font-weight: 600; }
.form-grid {
  display: grid;
  grid-template-columns: 110px 1fr;
  align-items: center;
  gap: 10px 12px;
  margin-bottom: 12px;
}
.form-grid label { font-size: 13px; color: var(--dw-text-2); }
.key-status { font-size: 12px; margin-bottom: 6px; }
.dim-note { font-size: 12px; margin-bottom: 10px; }
</style>
