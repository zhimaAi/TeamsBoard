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
        redirect: '/workbench',
      },
      // Workbench (the internal name follows goteams, consistent with the menu configuration key)
      {
        path: 'workbench',
        name: 'goteams',
        component: () => import('@/views/workbench/index.vue'),
        meta: { title: '工作台', menu: 'goteams' },
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
        name: 'workflows',
        component: () => import('@/views/workflows/TaskBoard.vue'),
        meta: { title: '任务看板', menu: 'workflows', requiresCloud: true },
      },
      {
        path: 'workflows/import',
        name: 'workflows-import',
        component: () => import('@/views/workflows/ImportTask.vue'),
        meta: { title: '导入需求 / 缺陷', menu: 'workflows', requiresCloud: true },
      },
      {
        path: 'workflows/task/:taskUuid',
        name: 'workflows-task-detail',
        component: () => import('@/views/workflows/TaskDetail.vue'),
        props: true,
        meta: { title: '任务详情', menu: 'workflows', requiresCloud: true },
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
    redirect: '/workbench',
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const authStore = useAuthStore()

  //Routes that require cloud login. If not logged in, return to the workbench to log in.
  if (to.meta.requiresCloud && !authStore.cloudLoggedIn) {
    return { path: '/workbench' }
  }

  return true
})

export default router
