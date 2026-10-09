<template>
  <aside
    v-show="!sidebarCollapsed"
    class="app-sidebar"
    :class="{ 'theme-dark': isDark, 'mac-desktop': isMacDesktop, 'windows-desktop': isWindowsDesktop }"
  >
    <div class="sidebar-top-bar">
      <button
        type="button"
        class="sidebar-collapse-btn"
        :aria-label="t('layout.sidebar.collapse')"
        :title="t('layout.sidebar.collapse')"
        @click="collapseSidebar"
      >
        <img
          :src="toggleIcon"
          alt=""
          class="collapse-icon"
        />
      </button>
    </div>

    <!-- 品牌行 -->
    <div class="sidebar-brand">
      <img
        class="brand-logo"
        :src="isDark ? logoDark : logo"
        alt="Logo"
      />
      <span class="brand-name">TeamsBoard</span>
    </div>

    <!-- 顶部主导航菜单 -->
    <nav class="sidebar-primary-nav" :aria-label="t('layout.sidebar.navigation')">
      <!-- 新对话 -->
      <RouterLink
        to="/tasks/new"
        class="nav-item new-conversation-item"
        :class="{ active: isNewConversationActive }"
      >
        <SidebarMenuIcon class="nav-icon" name="newConversation" :dark="isDark" />
        <span class="nav-label">{{ t('layout.sidebar.newConversation') || '新对话' }}</span>
      </RouterLink>

      <!-- 看板 -->
      <RouterLink
        to="/board"
        class="nav-item"
        :class="{ active: activeKey === 'workflows' }"
      >
        <SidebarMenuIcon class="nav-icon" name="dashboard" :dark="isDark" />
        <span class="nav-label">{{ t('layout.menu.workflows') }}</span>
      </RouterLink>

      <!-- Agent员工 -->
      <RouterLink
        to="/agents"
        class="nav-item"
        :class="{ active: activeKey === 'agents' }"
      >
        <SidebarMenuIcon class="nav-icon" name="agent" :dark="isDark" />
        <span class="nav-label">{{ t('layout.menu.agents') }}</span>
      </RouterLink>

      <!-- 知识库 -->
      <RouterLink
        to="/knowledge"
        class="nav-item"
        :class="{ active: activeKey === 'knowledge' }"
      >
        <SidebarMenuIcon class="nav-icon" name="book" :dark="isDark" />
        <span class="nav-label">{{ t('layout.menu.knowledge') }}</span>
      </RouterLink>

      <!-- 远程通道 -->
      <RouterLink
        to="/remote-channels"
        class="nav-item"
        :class="{ active: activeKey === 'remote-channels' }"
      >
        <SidebarMenuIcon class="nav-icon" name="remote" :dark="isDark" />
        <span class="nav-label">{{ t('layout.menu.remoteChannels') }}</span>
      </RouterLink>

      <!-- 更多菜单（悬停或点击展示项目、配置中心、命令、接口管理） -->
      <a-dropdown
        :trigger="['hover', 'click']"
        placement="bottomRight"
        overlay-class-name="sidebar-more-dropdown"
      >
        <div
          class="nav-item more-item"
          :class="{ active: isMoreMenuActive }"
        >
          <div class="more-item__left">
            <EllipsisOutlined class="nav-icon ellipsis-icon" />
            <span class="nav-label">{{ t('layout.sidebar.more') || '更多' }}</span>
          </div>
          <RightOutlined class="more-chevron" />
        </div>

        <template #overlay>
          <div class="more-menu-panel" role="menu">
            <RouterLink
              to="/projects"
              class="more-menu-item"
              :class="{ active: activeKey === 'projects' }"
            >
              <SidebarMenuIcon class="item-icon" name="project" :dark="isDark" />
              <span>{{ t('layout.menu.projects') }}</span>
            </RouterLink>
            <RouterLink
              to="/settings"
              class="more-menu-item"
              :class="{ active: activeKey === 'settings' }"
            >
              <SidebarMenuIcon class="item-icon" name="settings" :dark="isDark" />
              <span>{{ t('layout.menu.settings') }}</span>
            </RouterLink>
            <RouterLink
              to="/commands"
              class="more-menu-item"
              :class="{ active: activeKey === 'commands' }"
            >
              <SidebarMenuIcon class="item-icon" name="command" :dark="isDark" />
              <span>{{ t('layout.menu.commands') }}</span>
            </RouterLink>
            <RouterLink
              to="/apis"
              class="more-menu-item"
              :class="{ active: activeKey === 'apis' }"
            >
              <SidebarMenuIcon class="item-icon" name="api" :dark="isDark" />
              <span>{{ t('layout.menu.apis') }}</span>
            </RouterLink>
          </div>
        </template>
      </a-dropdown>
    </nav>

    <!-- 搜索对话框 -->
    <div class="sidebar-search">
      <div class="search-input-box">
        <SearchOutlined class="search-icon" />
        <input
          v-model="searchKeyword"
          type="text"
          class="search-input"
          :placeholder="t('layout.sidebar.searchConversations') || '搜索对话'"
          @input="onSearchInput"
        />
        <button
          v-if="searchKeyword"
          type="button"
          class="search-clear-btn"
          @click="clearSearch"
        >
          <CloseCircleFilled />
        </button>
      </div>
    </div>

    <!-- 对话列表区域（按工作目录分组，自适应高度，单独滚动） -->
    <div class="sidebar-conversations-scroll scrollbar--subtle">
      <div
        v-if="conversationStore.loading && !conversationStore.loaded"
        class="conversations-loading"
      >
        <a-spin size="small" />
      </div>

      <div
        v-else-if="conversationStore.error && !conversationStore.allConversations.length"
        class="conversations-error"
      >
        <span>{{ conversationStore.error }}</span>
        <button
          type="button"
          class="retry-btn"
          @click="conversationStore.loadConversations(true)"
        >
          {{ t('common.actions.retry') }}
        </button>
      </div>

      <!-- 未归档对话分组列表 -->
      <div
        v-for="group in conversationStore.activeGroups"
        :key="group.key"
        class="directory-group"
      >
        <div
          class="group-header"
          :title="group.fullPath"
          @click="toggleGroup(group.key)"
        >
          <div class="group-header__main">
            <RightOutlined
              class="group-chevron"
              :class="{ 'is-expanded': !conversationStore.isGroupCollapsed(group.key) }"
            />
            <FolderOutlined class="group-folder-icon" />
            <span class="group-name">{{ group.displayName }}</span>
          </div>

          <a-tooltip :title="t('layout.sidebar.addConversationInDir') || '在此目录新增对话'">
            <button
              type="button"
              class="group-add-btn"
              @click.stop="goToNewConversationWithDir(group.workDir)"
            >
              <PlusOutlined />
            </button>
          </a-tooltip>
        </div>

        <div
          v-show="!conversationStore.isGroupCollapsed(group.key)"
          class="group-conversations"
        >
          <div
            v-for="conv in group.items"
            :key="conv.task_uuid"
            class="conversation-entry"
            :class="{ active: isConversationActive(conv.task_uuid) }"
            @click="selectConversation(conv)"
          >
            <span
              class="conv-title"
              :title="conv.title"
            >{{ conv.title }}</span>

            <div class="conv-actions">
              <!-- 未读红点 -->
              <span
                v-if="conv.unread"
                class="unread-dot"
                :title="t('workflows.task.common.unread')"
              />

              <!-- 悬停更多操作按钮 -->
              <a-dropdown
                :trigger="['click']"
                placement="bottomRight"
              >
                <button
                  type="button"
                  class="conv-more-btn"
                  @click.stop
                >
                  <EllipsisOutlined />
                </button>
                <template #overlay>
                  <a-menu>
                    <a-menu-item @click="toggleReadState(conv)">
                      {{ conv.unread ? (t('workflows.task.common.markRead') || '标为已读') : (t('workflows.task.common.markUnread') || '设为未读') }}
                    </a-menu-item>
                    <a-menu-item @click="toggleArchiveState(conv)">
                      {{ t('workflows.task.common.archive') || '归档' }}
                    </a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>
            </div>
          </div>
        </div>
      </div>

      <!-- 已归档折叠组（默认收起） -->
      <div
        v-if="conversationStore.archivedConversations.length > 0"
        class="archived-section"
      >
        <div
          class="archived-header"
          @click="conversationStore.toggleArchivedCollapse"
        >
          <div class="archived-header__left">
            <RightOutlined
              class="group-chevron"
              :class="{ 'is-expanded': !conversationStore.archivedCollapsed }"
            />
            <InboxOutlined class="archived-icon" />
            <span class="archived-label">{{ t('layout.sidebar.archived') || '已归档' }}</span>
          </div>
          <span class="archived-count">{{ conversationStore.archivedConversations.length }}</span>
        </div>

        <div
          v-show="!conversationStore.archivedCollapsed"
          class="archived-content"
        >
          <div
            v-for="group in conversationStore.archivedGroups"
            :key="`archived-${group.key}`"
            class="directory-group archived-group"
          >
            <div
              class="group-header"
              :title="group.fullPath"
              @click="toggleGroup(`archived-${group.key}`)"
            >
              <div class="group-header__main">
                <RightOutlined
                  class="group-chevron"
                  :class="{ 'is-expanded': !conversationStore.isGroupCollapsed(`archived-${group.key}`) }"
                />
                <FolderOutlined class="group-folder-icon" />
                <span class="group-name">{{ group.displayName }}</span>
              </div>
            </div>

            <div
              v-show="!conversationStore.isGroupCollapsed(`archived-${group.key}`)"
              class="group-conversations"
            >
              <div
                v-for="conv in group.items"
                :key="conv.task_uuid"
                class="conversation-entry archived-entry"
                :class="{ active: isConversationActive(conv.task_uuid) }"
                @click="selectConversation(conv)"
              >
                <span
                  class="conv-title"
                  :title="conv.title"
                >{{ conv.title }}</span>

                <div class="conv-actions">
                  <a-dropdown
                    :trigger="['click']"
                    placement="bottomRight"
                  >
                    <button
                      type="button"
                      class="conv-more-btn"
                      @click.stop
                    >
                      <EllipsisOutlined />
                    </button>
                    <template #overlay>
                      <a-menu>
                        <a-menu-item @click="toggleReadState(conv)">
                          {{ conv.unread ? (t('workflows.task.common.markRead') || '标为已读') : (t('workflows.task.common.markUnread') || '设为未读') }}
                        </a-menu-item>
                        <a-menu-item @click="toggleArchiveState(conv)">
                          {{ t('layout.sidebar.unarchive') || '取消归档' }}
                        </a-menu-item>
                      </a-menu>
                    </template>
                  </a-dropdown>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 底部：用户信息与设置上滑菜单 -->
    <footer class="sidebar-footer">
      <div
        class="user-info-card"
        @click="handleUserClick"
      >
        <div class="user-avatar">
          <UserOutlined />
        </div>
        <div class="user-text">
          <span class="user-title">{{ authStore.cloudLoggedIn ? (authStore.cloudUser?.displayName || authStore.cloudUser?.username || '已登录') : (t('layout.sidebar.notLoggedIn') || '未登录') }}</span>
          <span class="user-sub">{{ authStore.cloudLoggedIn ? (authStore.cloudUser?.username || 'GoTeams') : (t('layout.sidebar.guestLogin') || '本地 · 点击登录') }}</span>
        </div>
      </div>

      <!-- 设置按钮（点击上滑弹出菜单） -->
      <a-dropdown
        v-model:open="settingsMenuOpen"
        :trigger="['click']"
        placement="topLeft"
        overlay-class-name="sidebar-settings-popover"
      >
        <button
          type="button"
          class="settings-btn"
          :class="{ 'is-open': settingsMenuOpen }"
          :aria-label="t('layout.menu.settings')"
        >
          <SettingOutlined />
        </button>

        <template #overlay>
          <div
            class="settings-up-menu"
            role="menu"
            @click.stop
          >
            <!-- 用户状态行 -->
            <div class="menu-user-row">
              <div class="user-avatar small">
                <UserOutlined />
              </div>
              <div class="user-text">
                <span class="user-title">{{ authStore.cloudLoggedIn ? (authStore.cloudUser?.displayName || authStore.cloudUser?.username) : (t('layout.sidebar.notLoggedIn') || '未登录') }}</span>
                <span class="user-sub">{{ authStore.cloudLoggedIn ? (authStore.cloudUser?.username || '') : (t('layout.sidebar.guestLogin') || '本地 · 点击登录') }}</span>
              </div>
            </div>

            <div class="menu-divider" />

            <!-- 外观切换菜单项 -->
            <a-dropdown
              placement="rightTop"
              :trigger="['hover', 'click']"
            >
              <div class="settings-menu-row">
                <div class="row-left">
                  <BulbOutlined class="row-icon" />
                  <span>{{ t('layout.sidebar.theme') || '外观' }}</span>
                </div>
                <div class="row-right">
                  <span class="current-val">{{ currentThemeLabel }}</span>
                  <RightOutlined class="chevron-right" />
                </div>
              </div>
              <template #overlay>
                <div class="sub-options-panel">
                  <button
                    type="button"
                    class="sub-option"
                    :class="{ 'is-selected': currentThemeConfig === 'light' }"
                    @click="chooseTheme('light')"
                  >
                    <span>{{ t('layout.sidebar.themeLight') || '浅色' }}</span>
                    <CheckOutlined v-if="currentThemeConfig === 'light'" class="check-icon" />
                  </button>
                  <button
                    type="button"
                    class="sub-option"
                    :class="{ 'is-selected': currentThemeConfig === 'dark' }"
                    @click="chooseTheme('dark')"
                  >
                    <span>{{ t('layout.sidebar.themeDark') || '深色' }}</span>
                    <CheckOutlined v-if="currentThemeConfig === 'dark'" class="check-icon" />
                  </button>
                  <button
                    type="button"
                    class="sub-option"
                    :class="{ 'is-selected': currentThemeConfig === 'auto' }"
                    @click="chooseTheme('auto')"
                  >
                    <span>{{ t('layout.sidebar.themeAuto') || '跟随系统' }}</span>
                    <CheckOutlined v-if="currentThemeConfig === 'auto'" class="check-icon" />
                  </button>
                </div>
              </template>
            </a-dropdown>

            <!-- 语言切换菜单项 -->
            <a-dropdown
              placement="rightTop"
              :trigger="['hover', 'click']"
            >
              <div class="settings-menu-row">
                <div class="row-left">
                  <GlobalOutlined class="row-icon" />
                  <span>{{ t('layout.sidebar.language') || '语言' }}</span>
                </div>
                <div class="row-right">
                  <span class="current-val">{{ currentLocaleLabel }}</span>
                  <RightOutlined class="chevron-right" />
                </div>
              </div>
              <template #overlay>
                <div class="sub-options-panel">
                  <button
                    type="button"
                    class="sub-option"
                    :class="{ 'is-selected': locale === 'zh-CN' }"
                    @click="chooseLocale('zh-CN')"
                  >
                    <span>{{ t('layout.sidebar.languageChinese') }}</span>
                    <CheckOutlined v-if="locale === 'zh-CN'" class="check-icon" />
                  </button>
                  <button
                    type="button"
                    class="sub-option"
                    :class="{ 'is-selected': locale === 'en-US' }"
                    @click="chooseLocale('en-US')"
                  >
                    <span>{{ t('layout.sidebar.languageEnglish') }}</span>
                    <CheckOutlined v-if="locale === 'en-US'" class="check-icon" />
                  </button>
                </div>
              </template>
            </a-dropdown>

            <button
              type="button"
              class="settings-menu-row"
              @click="reviewOnboarding"
            >
              <div class="row-left">
                <CompassOutlined class="row-icon" />
                <span>{{ t('layout.sidebar.onboarding') }}</span>
              </div>
            </button>

            <!-- 退出登录（如已登录） -->
            <template v-if="authStore.cloudLoggedIn">
              <div class="menu-divider" />
              <button
                type="button"
                class="settings-menu-row logout-row"
                @click="handleLogout"
              >
                <div class="row-left">
                  <LogoutOutlined class="row-icon" />
                  <span>{{ t('layout.sidebar.logout') || '退出登录' }}</span>
                </div>
              </button>
            </template>
          </div>
        </template>
      </a-dropdown>
    </footer>
  </aside>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import {
  EllipsisOutlined,
  RightOutlined,
  SearchOutlined,
  CloseCircleFilled,
  PlusOutlined,
  FolderOutlined,
  InboxOutlined,
  UserOutlined,
  SettingOutlined,
  BulbOutlined,
  GlobalOutlined,
  LogoutOutlined,
  CheckOutlined,
  CompassOutlined,
} from '@ant-design/icons-vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useConversationStore, type TaskConversation } from '@/stores/conversation'
import { useAppI18n } from '@/i18n'
import { useOnboardingGuide } from '@/composables/useOnboardingGuide'
import { useLocale } from '@/composables/useLocale'
import { useBrowserLogin } from '@/composables/useBrowserLogin'
import { isMacDesktopRuntime, isWindowsDesktopRuntime } from '@/composables/useDesktop'
import apiClient from '@/api/client'
import logo from '@/assets/logo.svg'
import logoDark from '@/assets/logo-dark.svg'
import sidebarToggleDark from '@/assets/icons/sidebar-toggle-dark.svg'
import sidebarToggleLight from '@/assets/icons/sidebar-toggle-light.svg'
import SidebarMenuIcon from './SidebarMenuIcon.vue'

type ThemeMode = 'light' | 'dark' | 'auto'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const conversationStore = useConversationStore()
const { t } = useAppI18n()
const { replay: replayOnboarding } = useOnboardingGuide()
const { locale, setLocale } = useLocale()
const {
  pending: browserLoginPending,
  errorMessage: browserLoginError,
  startBrowserLogin,
} = useBrowserLogin()

const searchKeyword = ref('')
const settingsMenuOpen = ref(false)

const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isMacDesktop = isMacDesktopRuntime()
const isWindowsDesktop = isWindowsDesktopRuntime()

function readThemePreference(): ThemeMode {
  try {
    const raw = localStorage.getItem('goteams.theme.mode')
    if (raw === 'light' || raw === 'dark' || raw === 'auto') return raw
    return 'light'
  } catch {
    return 'light'
  }
}

const currentThemeConfig = ref<ThemeMode>(readThemePreference())
const systemDark = ref(
  typeof window !== 'undefined' ? window.matchMedia('(prefers-color-scheme: dark)').matches : false,
)

let mediaQueryListener: ((e: MediaQueryListEvent) => void) | null = null

onMounted(() => {
  if (typeof window !== 'undefined') {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    mediaQueryListener = (e) => {
      systemDark.value = e.matches
    }
    mq.addEventListener('change', mediaQueryListener)
  }
})

onUnmounted(() => {
  if (mediaQueryListener && typeof window !== 'undefined') {
    window.matchMedia('(prefers-color-scheme: dark)').removeEventListener('change', mediaQueryListener)
  }
})

const isDark = computed(() => {
  if (currentThemeConfig.value === 'dark') return true
  if (currentThemeConfig.value === 'light') return false
  return systemDark.value
})

const toggleIcon = computed(() => (isDark.value ? sidebarToggleDark : sidebarToggleLight))

const currentThemeLabel = computed(() => {
  if (currentThemeConfig.value === 'light') return t('layout.sidebar.themeLight') || '浅色'
  if (currentThemeConfig.value === 'dark') return t('layout.sidebar.themeDark') || '深色'
  return t('layout.sidebar.themeAuto') || '跟随系统'
})

const currentLocaleLabel = computed(() => (locale.value === 'zh-CN' ? '中文' : 'English'))

const activeKey = computed(() => {
  const name = route.name as string | undefined
  if (!name) return ''
  if (name.startsWith('workflows') || name.startsWith('board')) return 'workflows'
  if (name.startsWith('apis')) return 'apis'
  if (name === 'remote-channels') return 'remote-channels'
  if (name === 'agents') return 'agents'
  if (name === 'projects') return 'projects'
  if (name === 'commands') return 'commands'
  if (name === 'knowledge') return 'knowledge'
  if (name === 'settings') return 'settings'
  return name
})

const isNewConversationActive = computed(() => {
  return route.name === 'tasks-new' || (route.path === '/tasks' && !route.query.taskUuid)
})

const isMoreMenuActive = computed(() => {
  return ['projects', 'settings', 'commands', 'apis'].includes(activeKey.value)
})

function isConversationActive(taskUuid: string): boolean {
  return route.name === 'tasks' && route.query.taskUuid === taskUuid
}

function collapseSidebar() {
  appStore.setSidebarCollapsed(true)
}

function chooseTheme(mode: ThemeMode) {
  currentThemeConfig.value = mode
  try {
    localStorage.setItem('goteams.theme.mode', mode)
    localStorage.setItem('goteams.sidebar.theme', isDark.value ? 'dark' : 'light')
  } catch {
    // ignore
  }
}

function chooseLocale(targetLocale: 'zh-CN' | 'en-US') {
  setLocale(targetLocale)
}

function reviewOnboarding() {
  replayOnboarding()
  settingsMenuOpen.value = false
  if (route.name !== 'tasks-new') {
    void router.push({ name: 'tasks-new' })
  }
}

function handleUserClick() {
  if (authStore.cloudLoggedIn) {
    void router.push({ path: '/board', query: { tab: 'team' } })
    return
  }
  if (browserLoginPending.value) return
  message.info(t('teamwork.login.pending'))
  void startBrowserLogin().then((ok) => {
    if (!ok && browserLoginError.value) message.error(browserLoginError.value)
  })
}

async function handleLogout() {
  try {
    await apiClient.post('/auth/logout')
  } catch {
    // ignore
  } finally {
    authStore.clearCloudAuth()
    authStore.setLocalSession(false)
    appStore.setCloudStatus('offline')
    settingsMenuOpen.value = false
    message.success(t('layout.sidebar.logoutSuccess') || '已退出登录')
  }
}

function onSearchInput() {
  conversationStore.setSearchQuery(searchKeyword.value)
}

function clearSearch() {
  searchKeyword.value = ''
  conversationStore.setSearchQuery('')
}

function toggleGroup(key: string) {
  conversationStore.toggleGroupCollapse(key)
}

function selectConversation(conv: TaskConversation) {
  if (conv.unread) {
    void conversationStore.setTaskRead(conv.task_uuid, true)
  }
  void router.push({ name: 'tasks', query: { taskUuid: conv.task_uuid } })
}

function goToNewConversationWithDir(dir: string) {
  void router.push({ name: 'tasks-new', query: dir ? { workDir: dir } : undefined })
}

async function toggleReadState(conv: TaskConversation) {
  try {
    await conversationStore.setTaskRead(conv.task_uuid, conv.unread)
    message.success(conv.unread ? (t('workflows.task.common.markRead') || '已标为已读') : (t('workflows.task.common.markUnread') || '已设为未读'))
  } catch (err) {
    message.error(err instanceof Error ? err.message : '操作失败')
  }
}

async function toggleArchiveState(conv: TaskConversation) {
  try {
    await conversationStore.archiveTask(conv.task_uuid, !conv.is_archived)
    message.success(conv.is_archived ? (t('layout.sidebar.unarchive') || '已取消归档') : (t('workflows.task.feedback.archived') || '已归档'))
  } catch (err) {
    message.error(err instanceof Error ? err.message : '操作失败')
  }
}

onMounted(() => {
  void conversationStore.loadConversations()
})
</script>

<style scoped>
.app-sidebar {
  --sidebar-bg: #f9f9f9;
  --sidebar-border: #f0f0f0;
  --sidebar-text: #262626;
  --sidebar-subtext: #8c8c8c;
  --sidebar-hover: #f0f0f0;
  --sidebar-active: #e9e9eb;
  --sidebar-item-radius: 8px;
  width: 280px;
  min-width: 280px;
  height: 100vh;
  background: var(--sidebar-bg);
  border-right: 1px solid var(--sidebar-border);
  display: flex;
  flex-direction: column;
  user-select: none;
  overflow: hidden;
  transition: background-color 0.2s;
}

.app-sidebar.theme-dark {
  --sidebar-bg: #0f172a;
  --sidebar-border: #1e293b;
  --sidebar-text: #f1f5f9;
  --sidebar-subtext: #94a3b8;
  --sidebar-hover: #1e293b;
  --sidebar-active: #334155;
}

.sidebar-top-bar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  height: 38px;
  flex-shrink: 0;
  padding: 0 12px;
}

.mac-desktop .sidebar-top-bar {
  -webkit-app-region: drag;
  justify-content: flex-start;
  padding-left: 80px;
}

.windows-desktop .sidebar-top-bar {
  -webkit-app-region: drag;
}

.sidebar-collapse-btn {
  -webkit-app-region: no-drag;
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background 0.15s;
}

.sidebar-collapse-btn:hover {
  background: var(--sidebar-hover);
}

.collapse-icon {
  width: 16px;
  height: 16px;
}

.sidebar-brand {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 44px;
  padding: 0 14px;
}

.brand-logo {
  width: 24px;
  height: 24px;
}

.brand-name {
  font-size: 17px;
  font-weight: 600;
  color: var(--sidebar-text);
  letter-spacing: -0.5px;
}

.sidebar-primary-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px 10px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 32px;
  padding: 0 10px;
  border-radius: var(--sidebar-item-radius);
  color: var(--sidebar-text);
  text-decoration: none;
  font-size: 14px;
  transition: background 0.15s, color 0.15s;
  cursor: pointer;
}

.nav-item:hover {
  background: var(--sidebar-hover);
}

.nav-item.active {
  background: var(--sidebar-active);
  font-weight: 500;
}

.nav-icon {
  font-size: 16px;
  flex-shrink: 0;
}

.nav-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.more-item {
  justify-content: space-between;
}

.more-item__left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.ellipsis-icon {
  font-size: 16px;
}

.more-chevron {
  font-size: 11px;
  color: var(--sidebar-subtext);
}

.more-menu-panel {
  width: 160px;
  background: #ffffff;
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  padding: 4px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  border: 1px solid #f0f0f0;
}

.more-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 10px;
  border-radius: 6px;
  color: #262626;
  text-decoration: none;
  font-size: 13px;
  transition: background 0.15s;
}

.more-menu-item:hover,
.more-menu-item.active {
  background: #f5f5f5;
}

.more-menu-item .item-icon {
  width: 16px;
  height: 16px;
}

.sidebar-search {
  padding: 8px 10px 4px;
}

.search-input-box {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 8px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  transition: border-color 0.2s;
}

.app-sidebar.theme-dark .search-input-box {
  background: #1e293b;
  border-color: #334155;
}

.search-input-box:focus-within {
  border-color: #3157e2;
}

.search-icon {
  color: #9ca3af;
  font-size: 13px;
}

.search-input {
  flex: 1;
  border: none;
  outline: none;
  background: transparent;
  font-size: 12px;
  color: var(--sidebar-text);
}

.search-input::placeholder {
  color: #9ca3af;
}

.search-clear-btn {
  border: none;
  background: transparent;
  color: #9ca3af;
  cursor: pointer;
  padding: 2px;
  display: flex;
  align-items: center;
}

.sidebar-conversations-scroll {
  flex: 1;
  overflow-y: auto;
  padding: 4px 10px 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.conversations-loading {
  padding: 24px;
  text-align: center;
}

.conversations-error {
  padding: 16px;
  font-size: 12px;
  color: #ef4444;
  text-align: center;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.retry-btn {
  border: none;
  background: transparent;
  color: #2563eb;
  cursor: pointer;
  text-decoration: underline;
}

.directory-group {
  display: flex;
  flex-direction: column;
}

.group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 28px;
  padding: 0 4px;
  cursor: pointer;
  border-radius: 6px;
  transition: background 0.15s;
}

.group-header:hover {
  background: var(--sidebar-hover);
}

.group-header__main {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  flex: 1;
}

.group-chevron {
  font-size: 10px;
  color: var(--sidebar-subtext);
  transition: transform 0.2s;
}

.group-chevron.is-expanded {
  transform: rotate(90deg);
}

.group-folder-icon {
  font-size: 14px;
  color: #595959;
}

.group-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--sidebar-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-add-btn {
  opacity: 0;
  width: 20px;
  height: 20px;
  border: none;
  background: transparent;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--sidebar-text);
  cursor: pointer;
  transition: opacity 0.15s, background 0.15s;
}

.group-header:hover .group-add-btn {
  opacity: 1;
}

.group-add-btn:hover {
  background: var(--sidebar-active);
}

.group-conversations {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding-left: 12px;
}

.conversation-entry {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 30px;
  padding: 0 8px;
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.15s;
  color: var(--sidebar-text);
}

.conversation-entry:hover {
  background: var(--sidebar-hover);
}

.conversation-entry.active {
  background: var(--sidebar-active);
  font-weight: 500;
}

.conv-title {
  flex: 1;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.conv-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.unread-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #fb363f;
}

.conv-more-btn {
  opacity: 0;
  width: 18px;
  height: 18px;
  border: none;
  background: transparent;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--sidebar-subtext);
  cursor: pointer;
  transition: opacity 0.15s;
}

.conversation-entry:hover .conv-more-btn {
  opacity: 1;
}

.conv-more-btn:hover {
  color: var(--sidebar-text);
}

.archived-section {
  margin-top: 6px;
  border-top: 1px solid var(--sidebar-border);
  padding-top: 6px;
}

.archived-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 28px;
  padding: 0 6px;
  cursor: pointer;
  border-radius: 6px;
  transition: background 0.15s;
}

.archived-header:hover {
  background: var(--sidebar-hover);
}

.archived-header__left {
  display: flex;
  align-items: center;
  gap: 6px;
}

.archived-icon {
  font-size: 13px;
  color: var(--sidebar-subtext);
}

.archived-label {
  font-size: 12px;
  color: var(--sidebar-subtext);
}

.archived-count {
  font-size: 11px;
  color: var(--sidebar-subtext);
}

.archived-content {
  padding-top: 4px;
}

.sidebar-footer {
  height: 52px;
  border-top: 1px solid var(--sidebar-border);
  padding: 0 10px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.user-info-card {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
  padding: 4px 6px;
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.15s;
}

.user-info-card:hover {
  background: var(--sidebar-hover);
}

.user-avatar {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: #e5e7eb;
  color: #4b5563;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  flex-shrink: 0;
}

.user-avatar.small {
  width: 24px;
  height: 24px;
  font-size: 12px;
}

.user-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.user-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--sidebar-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 16px;
}

.user-sub {
  font-size: 11px;
  color: var(--sidebar-subtext);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 14px;
}

.settings-btn {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #595959;
  font-size: 16px;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.settings-btn:hover,
.settings-btn.is-open {
  background: var(--sidebar-hover);
  color: var(--sidebar-text);
}

.settings-up-menu {
  width: 220px;
  background: #ffffff;
  border-radius: 10px;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.1);
  padding: 8px;
  border: 1px solid #f0f0f0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.menu-user-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 6px 8px;
}

.menu-divider {
  height: 1px;
  background: #f0f0f0;
  margin: 4px 0;
}

.settings-menu-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 32px;
  padding: 0 8px;
  border-radius: 6px;
  color: #262626;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s;
  border: none;
  background: transparent;
  width: 100%;
}

.settings-menu-row:hover {
  background: #f5f5f5;
}

.row-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.row-icon {
  font-size: 14px;
}

.row-right {
  display: flex;
  align-items: center;
  gap: 4px;
}

.current-val {
  font-size: 12px;
  color: #8c8c8c;
}

.chevron-right {
  font-size: 10px;
  color: #8c8c8c;
}

.logout-row {
  color: #ef4444;
}

.sub-options-panel {
  width: 140px;
  background: #ffffff;
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  padding: 4px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  border: 1px solid #f0f0f0;
}

.sub-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 30px;
  padding: 0 8px;
  border: none;
  background: transparent;
  border-radius: 4px;
  color: #262626;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s;
}

.sub-option:hover {
  background: #f5f5f5;
}

.sub-option.is-selected {
  background: #f0f7ff;
  color: #2563eb;
}

.check-icon {
  color: #2563eb;
  font-size: 12px;
}
</style>
