import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'repos', component: () => import('../views/ReposView.vue') },
    { path: '/ask/:repoId', name: 'ask', component: () => import('../views/AskView.vue') },
    { path: '/wiki/:repoId', name: 'wiki', component: () => import('../views/WikiView.vue') },
    { path: '/tasks', name: 'tasks', component: () => import('../views/TasksView.vue') },
    { path: '/system', name: 'system', component: () => import('../views/SystemView.vue') },
  ],
})

export default router
