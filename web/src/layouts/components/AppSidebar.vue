<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
// 本期去掉登录：团队工作跳转暂不使用 useRouter
// import { useRoute, useRouter } from 'vue-router'
import { useRoute } from 'vue-router'
// 本期去掉登录：账号/登录入口隐藏，相关图标暂不使用
// import { CloudOutlined, LogoutOutlined } from '@ant-design/icons-vue'
// import { message } from 'ant-design-vue'
import { useAppStore } from '@/stores/app'
import type { MenuConfigItem } from '@/stores/app'
// import { useAuthStore } from '@/stores/auth'
import apiClient from '@/api/client'
import { useAppI18n } from '@/i18n'
import { useLocale } from '@/composables/useLocale'
import sidebarLanguageDark from '@/assets/icons/sidebar-language-dark.svg'
import sidebarLanguageLight from '@/assets/icons/sidebar-language-light.svg'
// import sidebarLoginDarkCollapsed from '@/assets/icons/sidebar-login-dark-collapsed.svg'
// import sidebarLoginDarkExpanded from '@/assets/icons/sidebar-login-dark-expanded.svg'
// import sidebarLoginLightCollapsed from '@/assets/icons/sidebar-login-light-collapsed.svg'
// import sidebarLoginLightExpanded from '@/assets/icons/sidebar-login-light-expanded.svg'
import sidebarThemeDark from '@/assets/icons/sidebar-theme-dark.svg'
import sidebarThemeLight from '@/assets/icons/sidebar-theme-light.svg'
import sidebarToggleDark from '@/assets/icons/sidebar-toggle-dark.svg'
import sidebarToggleLight from '@/assets/icons/sidebar-toggle-light.svg'
import logo from '@/assets/logo.svg'
import logoDark from '@/assets/logo-dark.svg'
import SidebarMenuIcon from './SidebarMenuIcon.vue'

type SidebarTheme = 'light' | 'dark'

const sidebarCollapsedStorageKey = 'goteams.sidebar.collapsed'
const sidebarThemeStorageKey = 'goteams.sidebar.theme'

// 本期去掉登录：团队工作跳转暂不使用
// const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
const { t } = useAppI18n()
const { locale, currentOption, setLocale } = useLocale()
// const authStore = useAuthStore()

function readSidebarCollapsed() {
  try {
    return window.localStorage.getItem(sidebarCollapsedStorageKey) === 'true'
  } catch {
    return false
  }
}

function readSidebarTheme(): SidebarTheme {
  try {
    return window.localStorage.getItem(sidebarThemeStorageKey) === 'dark' ? 'dark' : 'light'
  } catch {
    return 'light'
  }
}

const sidebarCollapsed = ref(readSidebarCollapsed())
const sidebarTheme = ref<SidebarTheme>(readSidebarTheme())
// 本期去掉登录：退出登录入口隐藏，暂不使用
// const logoutLoading = ref(false)

const toggleIcon = computed(() => (
  sidebarTheme.value === 'dark' ? sidebarToggleDark : sidebarToggleLight
))
const themeIcon = computed(() => (
  sidebarTheme.value === 'dark' ? sidebarThemeDark : sidebarThemeLight
))
const languageIcon = computed(() => (
  sidebarTheme.value === 'dark' ? sidebarLanguageDark : sidebarLanguageLight
))
// 本期去掉登录：登录入口图标暂不使用
// const loginIcon = computed(() => {
//   if (sidebarTheme.value === 'dark') {
//     return sidebarCollapsed.value ? sidebarLoginDarkCollapsed : sidebarLoginDarkExpanded
//   }
//
//   return sidebarCollapsed.value ? sidebarLoginLightCollapsed : sidebarLoginLightExpanded
// })

const menuItems = [
  { key: 'tasks', labelKey: 'layout.menu.tasks', path: '/tasks', icon: 'task' },
  { key: 'workflows', labelKey: 'layout.menu.workflows', path: '/board', icon: 'dashboard' },
  { key: 'agents', labelKey: 'layout.menu.agents', path: '/agents', icon: 'agent' },
  { key: 'projects', labelKey: 'layout.menu.projects', path: '/projects', icon: 'project' },
  { key: 'commands', labelKey: 'layout.menu.commands', path: '/commands', icon: 'command' },
  { key: 'knowledge', labelKey: 'layout.menu.knowledge', path: '/knowledge', icon: 'book' },
  { key: 'apis', labelKey: 'layout.menu.apis', path: '/apis', icon: 'api' },
  { key: 'settings', labelKey: 'layout.menu.settings', path: '/settings', icon: 'settings' },
] as const

const visibleMenuItems = computed(() => {
  const order = new Map(appStore.menuOrder.map((key, index) => [key, index]))
  return [...menuItems]
    .sort((a, b) => (order.get(a.key) ?? menuItems.length) - (order.get(b.key) ?? menuItems.length))
    .filter((item) => appStore.isMenuVisible(item.key))
    .map((item) => ({ ...item, label: t(item.labelKey) }))
})

const activeKey = computed(() => {
  const name = route.name as string | undefined
  if (!name) return ''
  if (name.startsWith('workflows') || name.startsWith('board')) return 'workflows'
  if (name.startsWith('apis')) return 'apis'
  return menuItems.find((item) => item.key === name)?.key ?? ''
})

const unreadTaskCount = computed(() => appStore.unreadTaskNotifications)
const unreadTaskBadge = computed(() => {
  const count = unreadTaskCount.value
  if (count <= 0) return ''
  return count > 99 ? '99+' : String(count)
})

// 本期去掉登录：账号卡片隐藏，相关文案暂不使用
// const accountTitle = computed(() => (
//   authStore.cloudUser?.displayName || authStore.cloudUser?.username || '已登录'
// ))
//
// const accountSubtitle = computed(() => {
//   const username = authStore.cloudUser?.username
//   return username && username !== accountTitle.value ? username : '这里是账号'
// })

function persist(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value)
  } catch {
    // 浏览器禁用本地存储时，本次页面内的侧边栏状态仍然可用。
  }
}

function toggleSidebar() {
  sidebarCollapsed.value = !sidebarCollapsed.value
  persist(sidebarCollapsedStorageKey, String(sidebarCollapsed.value))
}

function toggleSidebarTheme() {
  sidebarTheme.value = sidebarTheme.value === 'light' ? 'dark' : 'light'
  persist(sidebarThemeStorageKey, sidebarTheme.value)
}

function toggleLocale() {
  setLocale(locale.value === 'zh-CN' ? 'en-US' : 'zh-CN')
}

// 本期去掉登录：团队工作跳转入口隐藏，暂不使用
// function goToTeamWorkspace() {
//   void router.push({ path: '/board', query: { tab: 'team' } })
// }
//
// async function handleLogout() {
//   if (logoutLoading.value) return
//   logoutLoading.value = true
//   try {
//     await apiClient.post('/auth/logout')
//   } catch {
//     // 请求失败时仍清理本地状态，避免失效账号继续留在侧边栏中。
//   } finally {
//     authStore.clearCloudAuth()
//     authStore.setLocalSession(false)
//     appStore.setCloudStatus('offline')
//     logoutLoading.value = false
//     message.success('已退出登录')
//   }
// }

function loadMenuConfig() {
  apiClient
    .get<{ items: MenuConfigItem[] }>('/tools/menu')
    .then((data) => {
      appStore.setMenuConfig(data?.items ?? [])
    })
    .catch(() => {
      // 加载失败时保留默认顺序和全部可见的菜单。
    })
}

onMounted(loadMenuConfig)
</script>

<template>
  <aside
    class="sidebar"
    :class="{
      collapsed: sidebarCollapsed,
      'theme-dark': sidebarTheme === 'dark',
    }"
  >
    <header class="sidebar-header">
      <div class="brand">
        <img
          class="brand-logo"
          :src="sidebarTheme === 'dark' ? logoDark : logo"
          alt=""
        >
        <span class="brand-name">TeamsBoard</span>
      </div>

      <button
        type="button"
        class="sidebar-toggle"
        :aria-expanded="!sidebarCollapsed"
        :aria-label="sidebarCollapsed ? t('layout.sidebar.expand') : t('layout.sidebar.collapse')"
        :title="sidebarCollapsed ? t('layout.sidebar.expand') : t('layout.sidebar.collapse')"
        @click="toggleSidebar"
      >
        <img
          class="sidebar-toggle-icon"
          :class="{ 'is-collapsed': sidebarCollapsed }"
          :src="toggleIcon"
          alt=""
        >
      </button>
    </header>

    <nav class="sidebar-nav" :aria-label="t('layout.sidebar.navigation')">
      <a-tooltip
        v-for="item in visibleMenuItems"
        :key="item.key"
        :title="sidebarCollapsed ? item.label : undefined"
        placement="right"
      >
        <RouterLink
          :to="item.path"
          class="nav-item"
          :class="{ active: activeKey === item.key }"
          :aria-current="activeKey === item.key ? 'page' : undefined"
        >
          <SidebarMenuIcon class="nav-icon" :name="item.icon" :dark="sidebarTheme === 'dark'" />
          <span class="nav-label">{{ item.label }}</span>
          <span
            v-if="item.key === 'tasks' && unreadTaskBadge"
            class="nav-unread-badge"
            :aria-label="t('workflows.task.common.unreadCount', { count: unreadTaskCount })"
          >{{ unreadTaskBadge }}</span>
        </RouterLink>
      </a-tooltip>
    </nav>

    <!-- 本期去掉登录：原 footer 绑定 :class="{ 'has-guest-account': !authStore.cloudLoggedIn }" -->
    <footer class="sidebar-footer">
      <!-- 本期去掉登录：账号/登录入口整体隐藏，恢复登录时取消注释
      <template v-if="authStore.cloudLoggedIn">
        <button
          type="button"
          class="account-card"
          :title="sidebarCollapsed ? `${accountTitle} · ${accountSubtitle}` : undefined"
          @click="goToTeamWorkspace"
        >
          <span class="account-icon" aria-hidden="true"><CloudOutlined /></span>
          <span class="account-copy">
            <strong>{{ accountTitle }}</strong>
            <small>{{ accountSubtitle }}</small>
          </span>
        </button>
        <button
          type="button"
          class="logout-button"
          :disabled="logoutLoading"
          aria-label="退出登录"
          title="退出登录"
          @click="handleLogout"
        >
          <LogoutOutlined />
        </button>
      </template>

      <button
        v-else
        type="button"
        class="account-card account-card-guest"
        :title="sidebarCollapsed ? '登录与团队一起协作' : undefined"
        @click="goToTeamWorkspace"
      >
        <span class="account-icon" aria-hidden="true">
          <img class="guest-account-icon" :src="loginIcon" alt="">
        </span>
        <span class="account-copy">
          <strong>登录与团队一起协作</strong>
          <small>本地模式 · 点击登录</small>
        </span>
      </button>
      -->

      <div class="sidebar-controls">
        <button
          type="button"
          class="sidebar-control"
          :aria-pressed="sidebarTheme === 'dark'"
          :aria-label="sidebarTheme === 'dark' ? t('layout.sidebar.switchToLight') : t('layout.sidebar.switchToDark')"
          :title="sidebarTheme === 'dark' ? t('layout.sidebar.switchToLight') : t('layout.sidebar.switchToDark')"
          @click="toggleSidebarTheme"
        >
          <img class="sidebar-control-icon theme-control-icon" :src="themeIcon" alt="">
          <span>{{ sidebarTheme === 'dark' ? t('layout.sidebar.dark') : t('layout.sidebar.light') }}</span>
        </button>
        <button
          type="button"
          class="sidebar-control language-control"
          :aria-label="t('layout.sidebar.currentLanguage', { language: currentOption.autonym })"
          :title="t('layout.sidebar.currentLanguage', { language: currentOption.autonym })"
          @click="toggleLocale"
        >
          <img class="sidebar-control-icon" :src="languageIcon" alt="">
          <span>{{ currentOption.shortLabel }}</span>
        </button>
      </div>
    </footer>
  </aside>
</template>

<style scoped>
.sidebar {
  --sidebar-bg: #f9f9f9;
  --sidebar-hover: #f0f1f3;
  --sidebar-active: #e9e9eb;
  --sidebar-text: #595959;
  --sidebar-text-strong: #1d1d1f;
  --sidebar-muted: #8c8c8c;
  --sidebar-toggle-bg: transparent;
  --sidebar-toggle-border: transparent;
  --sidebar-account-bg: transparent;
  position: relative;
  display: flex;
  width: 220px;
  min-width: 220px;
  height: 100vh;
  min-height: 0;
  flex-direction: column;
  overflow: visible;
  background: var(--sidebar-bg);
  color: var(--sidebar-text);
  transition: background-color 0.2s ease, color 0.2s ease;
}

.sidebar.theme-dark {
  --sidebar-bg: #0f172a;
  --sidebar-hover: #1e293b;
  --sidebar-active: #1e293b;
  --sidebar-text: #94a3b8;
  --sidebar-text-strong: #fff;
  --sidebar-muted: #94a3b8;
  --sidebar-toggle-bg: transparent;
  --sidebar-toggle-border: transparent;
  --sidebar-account-bg: transparent;
}

.sidebar.collapsed {
  width: 64px;
  min-width: 64px;
}

.sidebar-header {
  display: flex;
  height: 60px;
  min-height: 60px;
  align-items: center;
  justify-content: space-between;
  padding: 16px 12px;
}

.brand {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
}

.brand-logo {
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
}

.brand-name {
  overflow: hidden;
  color: var(--sidebar-text-strong);
  font-size: 18px;
  font-weight: 600;
  letter-spacing: -0.98px;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-toggle {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  padding: 6px;
  border: 1px solid var(--sidebar-toggle-border);
  border-radius: 10px;
  background: var(--sidebar-toggle-bg);
  color: var(--sidebar-text);
  cursor: pointer;
  transition: background-color 180ms ease, border-color 180ms ease;
}

.sidebar-toggle:hover,
.sidebar-toggle:focus-visible {
  background: var(--sidebar-hover);
  color: var(--sidebar-text-strong);
}

.sidebar-toggle-icon {
  display: block;
  width: 20px;
  height: 20px;
  transition: transform 180ms ease;
}

.sidebar-toggle-icon.is-collapsed {
  transform: rotate(180deg);
}

.sidebar-toggle:focus-visible,
.nav-item:focus-visible,
.account-card:focus-visible,
.logout-button:focus-visible,
.sidebar-control:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.sidebar-nav {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 2px;
  overflow-y: auto;
  padding: 0 10px;
}

.nav-item {
  position: relative;
  display: flex;
  min-height: 36px;
  align-items: center;
  gap: 12px;
  padding: 7px 12px;
  border-radius: 12px;
  color: var(--sidebar-text);
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
  text-decoration: none;
}

.nav-item:hover {
  background: var(--sidebar-hover);
  color: var(--sidebar-text-strong);
}

.nav-item.active {
  background: var(--sidebar-active);
  color: var(--sidebar-text-strong);
}

.nav-icon {
  display: inline-flex;
  width: 20px;
  height: 20px;
  min-width: 20px;
  align-items: center;
  justify-content: center;
}

.nav-label {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nav-unread-badge {
  display: inline-flex;
  min-width: 18px;
  height: 18px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  padding: 0 5px;
  color: #fff;
  background: #fb363f;
  font-size: 11px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  line-height: 18px;
}


.sidebar-footer {
  display: grid;
  flex: 0 0 auto;
  grid-template-columns: minmax(0, 1fr) 32px;
  gap: 8px;
  padding: 14px 10px 12px;
}

.account-card {
  display: flex;
  min-width: 0;
  height: 48px;
  align-items: center;
  gap: 10px;
  padding: 4px 8px;
  border: 0;
  border-radius: 12px;
  background: var(--sidebar-account-bg);
  color: var(--sidebar-text-strong);
  cursor: pointer;
  text-align: left;
}

.account-card-guest {
  grid-column: 1 / -1;
}

.account-card:hover {
  background: var(--sidebar-hover);
}

.account-icon {
  display: inline-flex;
  width: 36px;
  height: 36px;
  min-width: 36px;
  align-items: center;
  justify-content: center;
  border-radius: 18px;
  background: #f0f1f3;
  color: #8c8c8c;
  font-size: 20px;
}

.account-copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.account-copy strong,
.account-copy small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-copy strong {
  color: var(--sidebar-text-strong);
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
}

.account-copy small {
  color: var(--sidebar-muted);
  font-size: 12px;
  line-height: 18px;
}

.logout-button {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-self: center;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--sidebar-muted);
  cursor: pointer;
}

.logout-button:hover:not(:disabled) {
  background: var(--sidebar-hover);
  color: var(--sidebar-text-strong);
}

.logout-button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.sidebar-controls {
  display: contents;
}

.sidebar-control {
  display: inline-flex;
  height: 34px;
  min-width: 0;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 9px 8px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--sidebar-text);
  cursor: pointer;
  font-size: 12px;
  line-height: 16px;
}

.sidebar-control:hover {
  background: var(--sidebar-hover);
  color: var(--sidebar-text-strong);
}

.sidebar-control-icon {
  display: block;
  width: 16px;
  height: 16px;
}

.theme-control-icon {
  width: 14px;
  height: 14px;
}

.language-control {
  cursor: pointer;
}

.sidebar-footer > .sidebar-controls {
  grid-column: 1 / -1;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.sidebar-footer.has-guest-account {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0;
  padding: 0;
}

.sidebar-footer.has-guest-account .account-card-guest {
  box-sizing: border-box;
  width: 200px;
  height: 70px;
  padding: 14px 8px 8px;
}

.sidebar-footer.has-guest-account > .sidebar-controls {
  display: grid;
  width: 100%;
  height: 54px;
  flex: 0 0 54px;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  padding: 8px 12px 12px;
}

.guest-account-icon {
  display: block;
  width: 24px;
  height: 24px;
}

.theme-dark .account-card-guest .account-icon {
  background: #1e293b;
}

.sidebar.collapsed .sidebar-header {
  justify-content: center;
  padding: 0;
}

.sidebar.collapsed .brand-name,
.sidebar.collapsed .nav-label,
.sidebar.collapsed .account-copy,
.sidebar.collapsed .sidebar-control > span {
  display: none;
}

.sidebar.collapsed .sidebar-toggle {
  position: absolute;
  top: 14px;
  right: -18px;
  z-index: 1;
  border-color: #f0f0f0;
  border-radius: 32px;
  background: var(--sidebar-bg);
}

.sidebar.collapsed .sidebar-nav {
  padding: 0 10px;
}

.sidebar.collapsed .nav-item {
  justify-content: center;
  padding-right: 0;
  padding-left: 0;
}

.sidebar.collapsed .nav-unread-badge {
  position: absolute;
  top: 2px;
  right: 2px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  font-size: 10px;
  line-height: 16px;
}

.sidebar.collapsed .sidebar-footer {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 14px 10px 12px;
}

.sidebar.collapsed .account-card,
.sidebar.collapsed .logout-button,
.sidebar.collapsed .sidebar-control {
  width: 44px;
}

.sidebar.collapsed .account-card {
  justify-content: center;
  padding: 4px;
}

.sidebar.collapsed .sidebar-controls,
.sidebar.collapsed .sidebar-footer > .sidebar-controls {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.sidebar.collapsed .sidebar-control {
  min-height: 32px;
  padding: 8px;
}

.sidebar.collapsed .logout-button {
  order: 1;
}

.sidebar.collapsed .sidebar-footer.has-guest-account {
  height: 128px;
  padding: 0 10px 8px;
  gap: 8px;
}

.sidebar.collapsed .sidebar-footer.has-guest-account .account-card-guest {
  width: 44px;
  height: 40px;
  padding: 4px 6px;
}

.sidebar.collapsed .account-card-guest .account-icon {
  width: 32px;
  height: 32px;
  min-width: 32px;
  border-radius: 16px;
}

.sidebar.collapsed .guest-account-icon {
  width: 21.3333px;
  height: 21.3333px;
}

.sidebar.collapsed .sidebar-footer.has-guest-account > .sidebar-controls {
  display: flex;
  width: 44px;
  height: 72px;
  flex: 0 0 72px;
  flex-direction: column;
  gap: 8px;
  padding: 0;
}

.sidebar.collapsed .sidebar-footer.has-guest-account .sidebar-control {
  width: 44px;
  height: 32px;
  min-height: 32px;
  padding: 8px;
}
</style>
