import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    component: () => import('@/layouts/MainLayout.vue'),
    children: [
      {
        path: '',
        name: 'home',
        redirect: '/board',
      },
      {
        path: 'workbench',
        // 本期去掉登录：不再跳转团队工作（入口已禁用），直接进入本地看板
        // redirect: { path: '/board', query: { tab: 'team' } },
        redirect: '/board',
      },
      // Order
      {
        path: 'commands',
        name: 'commands',
        component: () => import('@/views/commands/Commands.vue'),
        meta: { title: '命令概览', menu: 'commands' },
      },
      {
        path: 'commands/git',
        redirect: '/commands',
      },
      {
        path: 'commands/docker',
        redirect: '/commands',
      },
      {
        path: 'commands/history',
        redirect: '/commands',
      },
      // Workflow
      {
        path: 'workflows',
        redirect: '/board',
      },
      {
        path: 'board',
        name: 'board',
        component: () => import('@/views/workflows/BoardHub.vue'),
        meta: { title: '看板', menu: 'workflows', contentLayout: 'custom', isScroll: false },
      },
      {
        path: 'workflows/import',
        name: 'workflows-import',
        component: () => import('@/views/workflows/ImportTask.vue'),
        meta: { title: '导入任务', menu: 'workflows', requiresCloud: true },
      },
      {
        path: 'workflows/task/:taskUuid',
        name: 'workflows-task-detail',
        component: () => import('@/views/workflows/TaskDetail.vue'),
        props: true,
        meta: { title: '任务详情', menu: 'workflows' },
      },
      {
        path: 'board/task/:taskUuid',
        name: 'board-task-detail',
        component: () => import('@/views/workflows/TaskDetail.vue'),
        props: true,
        meta: { title: '任务详情', menu: 'workflows',contentLayout: 'custom', isScroll: false},
      },
      {
        path: 'tasks',
        name: 'tasks',
        component: () => import('@/views/tasks/TaskNotifications.vue'),
        meta: { title: '对话', menu: 'tasks', contentLayout: 'custom', isScroll: false },
      },
      {
        path: 'agents',
        name: 'agents',
        component: () => import('@/views/agents/AgentPipelines.vue'),
        meta: { title: '专家流水线', menu: 'agents', keepAlive: true },
      },
      {
        path: 'projects',
        name: 'projects',
        component: () => import('@/views/projects/Projects.vue'),
        meta: { title: '项目', menu: 'projects', keepAlive: true },
      },
      // knowledge base
      {
        path: 'knowledge',
        name: 'knowledge',
        component: () => import('@/views/knowledge/KnowledgeBase.vue'),
        meta: { title: '知识库', menu: 'knowledge' },
      },
      //Interface management
      {
        path: 'apis',
        name: 'apis',
        component: () => import('@/views/apis/ApiManage.vue'),
        meta: { title: '接口管理', menu: 'apis' },
      },
      // Configuration center (first-level menu, use tab to switch sub-items within the page)
      {
        path: 'settings',
        name: 'settings',
        component: () => import('@/views/settings/Settings.vue'),
        meta: { title: '配置中心', menu: 'settings' },
      },
      // Compatible with old deep links
      { path: 'settings/git', redirect: '/settings' },
      { path: 'settings/ssh', redirect: '/settings' },
      { path: 'settings/docker', redirect: '/settings' },
      { path: 'settings/database', redirect: '/settings' },
      { path: 'settings/models', redirect: '/settings' },
      // Tool Center
      {
        path: 'tools',
        name: 'tools',
        component: () => import('@/views/tools/ToolsCenter.vue'),
        meta: { title: '工具中心', menu: 'tools' },
      },
    ],
  },
  // The bottom line
  {
    path: '/:pathMatch(.*)*',
    redirect: '/board',
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const authStore = useAuthStore()

  // 本期去掉登录：登录入口已隐藏，未登录访问 requiresCloud 路由时回到本地看板（不再进入登录页）
  if (to.meta.requiresCloud && !authStore.cloudLoggedIn) {
    // return { path: '/workbench' }
    return { path: '/board' }
  }

  return true
})

router.afterEach((to) => {
  if (import.meta.env.DEV) {
    console.log(`[Route] ${to.fullPath}`)
  }
})

export default router
