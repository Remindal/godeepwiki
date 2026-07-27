<template>
  <div class="layout">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-logo">DW</div>
        <span class="brand-name">DeepWiki</span>
      </div>

      <button class="new-chat-btn" @click="$router.push({ path: '/', query: { ingest: '1' } })">
        <span>＋</span> 摄取仓库
      </button>

      <nav class="nav">
        <div
          v-for="item in navItems"
          :key="item.path"
          class="nav-item"
          :class="{ active: isActive(item.path) }"
          @click="$router.push(item.path)"
        >
          <span class="nav-dot" />
          {{ item.label }}
        </div>
      </nav>

      <div class="sidebar-footer">
        <div class="nav-item" :class="{ active: isActive('/system') }" @click="$router.push('/system')">
          <span class="nav-dot" /> 系统状态
        </div>
      </div>
    </aside>

    <main class="main">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router'

const route = useRoute()
const navItems = [
  { path: '/', label: '仓库' },
  { path: '/tasks', label: '任务中心' },
]
const isActive = (path: string) =>
  path === '/' ? route.path === '/' || route.path.startsWith('/ask') || route.path.startsWith('/wiki') : route.path.startsWith(path)
</script>

<style scoped>
.layout {
  display: flex;
  height: 100%;
  background: var(--dw-bg);
}

.sidebar {
  width: 232px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  padding: 16px 12px;
  background: var(--dw-bg-soft);
  border-right: 1px solid var(--dw-border);
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 8px 16px;
}
.brand-logo {
  width: 30px;
  height: 30px;
  border-radius: 9px;
  background: var(--dw-black);
  color: var(--dw-white);
  font-size: 13px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  letter-spacing: 0.5px;
}
.brand-name { font-weight: 600; font-size: 15px; }

.new-chat-btn {
  width: 100%;
  padding: 9px 12px;
  margin-bottom: 14px;
  border: 1px solid var(--dw-border);
  border-radius: var(--dw-radius-md);
  background: var(--dw-white);
  color: var(--dw-text);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  text-align: left;
  transition: box-shadow 0.15s ease;
}
.new-chat-btn:hover { box-shadow: var(--dw-shadow); }
.new-chat-btn span { margin-right: 4px; }

.nav { flex: 1; display: flex; flex-direction: column; gap: 2px; }

.nav-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: var(--dw-radius-sm);
  font-size: 13px;
  color: var(--dw-text-2);
  cursor: pointer;
  transition: background 0.12s ease;
  user-select: none;
}
.nav-item:hover { background: var(--dw-bg-mute); }
.nav-item.active {
  background: var(--dw-white);
  color: var(--dw-text);
  font-weight: 500;
  box-shadow: var(--dw-shadow);
}
.nav-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.5;
}

.sidebar-footer { padding-top: 10px; border-top: 1px solid var(--dw-border); }

.main {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
}
</style>
