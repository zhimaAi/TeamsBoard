<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons-vue'
import { useAppStore } from '@/stores/app'
import type { MenuConfigItem } from '@/stores/app'
import { useLocalWS } from '@/composables/useLocalWebSocket'
import { provideDocumentTitle } from '@/composables/useDocumentTitle'
import MenuIcon from '@/components/MenuIcon.vue'
import apiClient from '@/api/client'

const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
const sidebarCollapsedStorageKey = 'goteams.sidebar.collapsed'

function readSidebarCollapsed() {
  try {
    return window.localStorage.getItem(sidebarCollapsedStorageKey) === 'true'
  } catch {
    return false
  }
}

const sidebarCollapsed = ref(readSidebarCollapsed())

function toggleSidebar() {
  sidebarCollapsed.value = !sidebarCollapsed.value
  try {
    window.localStorage.setItem(sidebarCollapsedStorageKey, String(sidebarCollapsed.value))
  } catch {
    // 浏览器禁用本地存储时，当前页面内仍保持可正常收起。
  }
}

/** Menu items whose visibility and order can be adjusted in the tool center */
const menuItems = [
  { key: 'goteams', label: '工作台', path: '/workbench', icon: 'dashboard' },
  { key: 'workflows', label: '工作流程', path: '/workflows', icon: 'workflow' },
  { key: 'commands', label: '命令', path: '/commands', icon: 'command' },
  { key: 'knowledge', label: '知识库', path: '/knowledge', icon: 'book' },
  { key: 'apis', label: '接口管理', path: '/apis', icon: 'api' },
  { key: 'settings', label: '配置中心', path: '/settings', icon: 'settings' },
]

/** The tool center is not part of sorting/switching and is fixed at the bottom of the sidebar */
const toolsMenuItem = { key: 'tools', label: '工具中心', path: '/tools', icon: 'toolbox' }

const currentMenuTitle = computed(() => {
  const menuKey = route.meta.menu
  return [...menuItems, toolsMenuItem].find((item) => item.key === menuKey)?.label
    || route.meta.title
})

provideDocumentTitle(currentMenuTitle)

/** Sort by storage order, then filter by switch state */
const visibleMenuItems = computed(() => {
  const order = new Map(appStore.menuOrder.map((key, index) => [key, index]))
  return [...menuItems]
    .sort((a, b) => (order.get(a.key) ?? menuItems.length) - (order.get(b.key) ?? menuItems.length))
    .filter((item) => appStore.isMenuVisible(item.key))
})

/** Currently selected menu key */
const activeKey = computed(() => {
  const name = route.name as string | undefined
  if (!name) return ''
  // The workflows sub-route also highlights the workflow entry
  if (name.startsWith('workflows')) return 'workflows'
  if (name.startsWith('apis')) return 'apis'
  const item = [...menuItems, toolsMenuItem].find((m) => m.key === name)
  return item ? item.key : ''
})

/** Menu click navigation */
function handleMenuClick(path: string) {
  router.push(path)
}

/** Load sidebar menu switch configuration */
function loadMenuConfig() {
  apiClient
    .get<{ items: MenuConfigItem[] }>('/tools/menu')
    .then((data) => {
      appStore.setMenuConfig(data?.items ?? [])
    })
    .catch(() => {
      // On load failure, keep defaults (all visible)
    })
}

/** Cloud status */
const cloudOnline = computed(() => appStore.cloudStatus === 'online')
const cloudStatusText = computed(() => {
  switch (appStore.cloudStatus) {
    case 'online':
      return '在线'
    case 'auth-expired':
      return '认证失效'
    default:
      return '离线'
  }
})

/** Receive the count of globally running CLI tasks via WebSocket push (backend pushes every 5s) */
useLocalWS('app.cli_count', (data: { count: number }) => {
  appStore.setCliTaskCount(data?.count ?? 0)
})

/** Cloud WebSocket connection state changes (online/offline), pushed by the backend on connect/disconnect */
useLocalWS('cloud.status', (data: { status: 'online' | 'offline' | 'auth-expired' }) => {
  appStore.setCloudStatus(data?.status ?? 'offline')
})

onMounted(loadMenuConfig)
</script>

<template>
  <div class="app-layout">
    <!-- Dark sidebar 220px -->
    <aside class="sidebar" :class="{ collapsed: sidebarCollapsed }">
      <!-- Top logo -->
      <div class="sidebar-header">
        <div class="logo-icon"><img src="@/assets/logo-dark.svg" alt=""></div>
        <span class="logo-text">Teams Desk Agent</span>
      </div>

      <button
        type="button"
        class="sidebar-toggle"
        :aria-label="sidebarCollapsed ? '展开导航菜单' : '收起导航菜单'"
        :title="sidebarCollapsed ? '展开导航菜单' : '收起导航菜单'"
        @click="toggleSidebar"
      >
        <MenuUnfoldOutlined v-if="sidebarCollapsed" />
        <MenuFoldOutlined v-else />
      </button>

      <!-- Navigation menu -->
      <nav class="sidebar-nav">
        <a-tooltip
          v-for="item in visibleMenuItems"
          :key="item.key"
          :title="sidebarCollapsed ? item.label : undefined"
          placement="right"
        >
          <div
            class="nav-item"
            :class="{ active: activeKey === item.key }"
            @click="handleMenuClick(item.path)"
          >
            <MenuIcon class="nav-icon" :name="item.icon" />
            <span class="nav-label">{{ item.label }}</span>
          </div>
        </a-tooltip>
      </nav>

      <!-- Tool center is always fixed between the normal menu and the status bar -->
      <div class="sidebar-tools">
        <a-tooltip
          :title="sidebarCollapsed ? toolsMenuItem.label : undefined"
          placement="right"
        >
          <div
            class="nav-item"
            :class="{ active: activeKey === toolsMenuItem.key }"
            @click="handleMenuClick(toolsMenuItem.path)"
          >
            <MenuIcon class="nav-icon" :name="toolsMenuItem.icon" />
            <span class="nav-label">{{ toolsMenuItem.label }}</span>
          </div>
        </a-tooltip>
      </div>

      <!-- Bottom status -->
      <div
        class="sidebar-footer"
        :title="sidebarCollapsed ? `${cloudStatusText} | CLI 任务: ${appStore.cliTaskCount}` : undefined"
      >
        <span class="status-dot" :class="{ online: cloudOnline }" />
        <span class="status-text">{{ cloudStatusText }}</span>
        <span class="footer-divider">|</span>
        <span class="status-text">CLI 任务: {{ appStore.cliTaskCount }}</span>
      </div>
    </aside>

    <!-- Main content area -->
    <main
      class="main-content"
      :class="{
        'api-content': activeKey === 'apis',
        'knowledge-content': activeKey === 'knowledge',
      }"
    >
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.app-layout {
  display: flex;
  height: 100vh;
  overflow: hidden;
}

/* ===== Dark sidebar ===== */
.sidebar {
  position: relative;
  width: 220px;
  min-width: 220px;
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: #0f172a;
  transition: width 0.2s, min-width 0.2s;
}

.sidebar.collapsed {
  width: 64px;
  min-width: 64px;
}

/* Logo area */
.sidebar-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 20px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.logo-icon {
  width: 32px;
  height: 32px;
  min-width: 32px;
  border-radius: 50%;
  color: #fff;
  font-size: 18px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}

.logo-text {
  font-size: 16px;
  font-weight: 600;
  color: #fff;
  white-space: nowrap;
}

.sidebar-toggle {
  position: absolute;
  z-index: 1;
  top: 24px;
  right: -12px;
  width: 24px;
  height: 24px;
  padding: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(255, 255, 255, 0.22);
  border-radius: 50%;
  background: #0f172a;
  color: rgba(255, 255, 255, 0.75);
  cursor: pointer;
  transition: color 0.2s, background 0.2s;
}

.sidebar-toggle:hover {
  color: #fff;
  background: #1e293b;
}

.sidebar-toggle:focus-visible {
  outline: 2px solid #fff;
  outline-offset: 2px;
}

.sidebar.collapsed .sidebar-header {
  justify-content: center;
  padding-right: 12px;
  padding-left: 12px;
}

.sidebar.collapsed .logo-text,
.sidebar.collapsed .nav-label,
.sidebar.collapsed .status-text,
.sidebar.collapsed .footer-divider {
  display: none;
}

/* Navigation menu */
.sidebar-nav {
  flex: 1;
  overflow-y: auto;
  padding: 12px 0;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 2px 8px;
  padding: 9px 12px;
  border-radius: 6px;
  color: rgba(255, 255, 255, 0.65);
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s;
  user-select: none;
}

.nav-item:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.08);
}

.nav-item.active {
  color: #fff;
  background: #3157e2;
}

.nav-icon {
  width: 18px;
  height: 18px;
  min-width: 18px;
}

.nav-label {
  white-space: nowrap;
}

.sidebar.collapsed .nav-item {
  justify-content: center;
  gap: 0;
  padding-right: 0;
  padding-left: 0;
}

/* Bottom status bar */
.sidebar-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 20px;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  font-size: 13px;
  color: rgba(255, 255, 255, 0.65);
  white-space: nowrap;
}

.sidebar.collapsed .sidebar-footer {
  justify-content: center;
  padding-right: 0;
  padding-left: 0;
}

.status-dot {
  width: 8px;
  height: 8px;
  min-width: 8px;
  border-radius: 50%;
  background: #666;
}

.status-dot.online {
  background: #52c41a;
}

.footer-divider {
  color: rgba(255, 255, 255, 0.25);
}

/* ===== Main content area ===== */
.main-content {
  flex: 1;
  height: 100vh;
  overflow-y: auto;
  background: #f5f6f8;
  padding: 24px;
}

/* API management is an immersive three-pane workbench; the design does not keep generic page padding. */
.main-content.api-content {
  overflow: hidden;
  background: #fff;
  padding: 0;
}

.sidebar-tools {
  flex-shrink: 0;
  padding: 8px 0;
}

.main-content.knowledge-content {
  overflow: hidden;
  background: #fff;
  padding: 0;
}
</style>
