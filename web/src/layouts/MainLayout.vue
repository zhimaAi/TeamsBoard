<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { notification } from 'ant-design-vue'
import apiClient from '@/api/client'
import { useAppStore } from '@/stores/app'
import {
  isDesktopRuntime,
  onOpenTaskConversation,
  showTaskNotification,
  syncTrayTaskCount,
} from '@/composables/useDesktop'
import { useLocalWS } from '@/composables/useLocalWebSocket'
import { provideDocumentTitle } from '@/composables/useDocumentTitle'
import { useAppI18n } from '@/i18n'
import type { TaskWithDetails } from '@/types/task-detail'
import type { TaskNotification } from '@/types/pipeline'
import AppSidebar from './components/AppSidebar.vue'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const { t } = useAppI18n()
const notificationPermission = ref<NotificationPermission>(
  typeof window !== 'undefined' && 'Notification' in window ? Notification.permission : 'denied',
)
const seenNotificationIds = new Set<string>()
const notificationFetchVersion = ref(0)
const MAX_REMEMBERED_NOTIFICATIONS = 500
const notifiedSessionUuids = new Set<string>()
const notifyingSessionUuids = new Set<string>()

const currentMenuTitle = computed(() =>
  route.meta.titleKey
    ? t(route.meta.titleKey)
    : typeof route.meta.title === 'string'
      ? route.meta.title
      : undefined,
)
provideDocumentTitle(currentMenuTitle)

const mainContentClasses = computed(() => {
  const layout = route.meta.contentLayout ?? 'default'
  const isScroll = route.meta.isScroll !== false
  const menu = route.meta.menu

  return {
    [`main-content-${layout}`]: true,
    'main-content-scroll': isScroll,
    'main-content-no-scroll': !isScroll,
    'api-content': menu === 'apis',
    'agents-content': menu === 'agents',
    'knowledge-content': menu === 'knowledge',
  }
})

// 侧边栏不再展示任务计数，但仍保持全局状态更新，供现有其他消费方继续使用。
useLocalWS('app.cli_count', (data: { count: number }) => {
  appStore.setCliTaskCount(data?.count ?? 0)
})

useLocalWS('app.notifications.unread', (data: { count: number }) => {
  const nextCount = data?.count ?? 0
  const previousCount = appStore.unreadTaskNotifications
  appStore.setUnreadTaskNotifications(nextCount)
  void syncTaskNotifications(previousCount, nextCount)
})

useLocalWS(
  'executor.completed',
  (data: { task_uuid?: string; step_key?: string; session_uuid?: string; status?: string }) => {
    if (!isDesktopRuntime() || data.status !== 'success' || !data.task_uuid || !data.session_uuid) {
      return
    }
    if (
      notifiedSessionUuids.has(data.session_uuid) ||
      notifyingSessionUuids.has(data.session_uuid)
    ) {
      return
    }
    notifyingSessionUuids.add(data.session_uuid)
    void notifyTaskReady(data.task_uuid, data.step_key || '', data.session_uuid)
  },
)

async function notifyTaskReady(taskUuid: string, stepKey: string, sessionUuid: string) {
  try {
    const task = await apiClient.get<TaskWithDetails>(`/tasks/${encodeURIComponent(taskUuid)}`)
    const step = task.steps.find((item) => item.step_key === stepKey || item.uuid === stepKey)
    const result = await showTaskNotification({
      title: t('layout.notifications.readyTitle', { step: step?.name || 'Agent' }),
      body: t('layout.notifications.openTask', { task: task.title }),
      taskUuid,
      stepUuid: step?.uuid,
    })
    if (result.shown || result.reason === 'unsupported') {
      notifiedSessionUuids.add(sessionUuid)
      if (notifiedSessionUuids.size > MAX_REMEMBERED_NOTIFICATIONS) {
        const oldestSessionUuid = notifiedSessionUuids.values().next().value
        if (oldestSessionUuid) notifiedSessionUuids.delete(oldestSessionUuid)
      }
    } else {
      notification.warning({
        key: 'desktop-notification-unavailable',
        message: t('layout.notifications.unavailableTitle'),
        description: result.message || t('layout.notifications.unavailableHint'),
        duration: 0,
      })
    }
  } catch {
    notification.warning({
      key: 'desktop-notification-unavailable',
      message: t('layout.notifications.sendFailedTitle'),
      description: t('layout.notifications.sendFailedHint'),
      duration: 0,
    })
  } finally {
    notifyingSessionUuids.delete(sessionUuid)
  }
}

watch(() => appStore.cliTaskCount, syncTrayTaskCount, { immediate: true })

async function syncTaskNotifications(previousCount: number, nextCount: number) {
  if (typeof window === 'undefined' || !('Notification' in window)) return
  if (route.name === 'tasks') return
  if (notificationPermission.value === 'default') {
    notificationPermission.value = await Notification.requestPermission()
  }
  if (notificationPermission.value !== 'granted' || nextCount <= previousCount) {
    return
  }
  const requestVersion = ++notificationFetchVersion.value
  try {
    const result = await apiClient.get<{ items: TaskNotification[] }>('/notifications')
    if (requestVersion !== notificationFetchVersion.value) return
    const items = (result.items || []).filter((item) => !item.is_read)
    for (const item of items) {
      if (seenNotificationIds.has(item.uuid)) continue
      seenNotificationIds.add(item.uuid)
      const title = t('layout.notifications.readyTitle', { step: item.step_name || 'Agent' })
      const body = t('layout.notifications.openTask', {
        task: item.task_title || t('layout.notifications.details'),
      })
      const notification = new Notification(title, { body })
      notification.onclick = () => {
        window.focus()
        void router.push({
          name: 'tasks',
          query: { taskUuid: item.task_uuid, stepUuid: item.task_step_uuid },
        })
      }
    }
  } catch {
    // 通知弹窗失败不影响页面主流程
  }
}

async function seedNotificationHistory() {
  try {
    const result = await apiClient.get<{ items: TaskNotification[] }>('/notifications')
    appStore.setUnreadTaskNotifications((result.items || []).filter((item) => !item.is_read).length)
    for (const item of result.items || []) {
      seenNotificationIds.add(item.uuid)
    }
  } catch {
    // 初始化失败不影响页面使用，后续 unread 推送会再触发。
  }
  // 侧栏未读状态不依赖系统通知权限，避免等待授权时红点迟迟不显示。
  if (typeof window !== 'undefined' && 'Notification' in window && notificationPermission.value === 'default') {
    notificationPermission.value = await Notification.requestPermission()
  }
}

onMounted(() => {
  void seedNotificationHistory()
})

let trayStatusHeartbeat: ReturnType<typeof setInterval> | null = null
let removeOpenTaskListener: (() => void) | null = null
onMounted(() => {
  removeOpenTaskListener = onOpenTaskConversation(({ taskUuid, stepUuid }) => {
    void router.push({
      name: 'tasks',
      query: stepUuid ? { taskUuid, stepUuid } : { taskUuid },
    })
  })
  trayStatusHeartbeat = setInterval(() => {
    syncTrayTaskCount(appStore.cliTaskCount)
  }, 20_000)
})
onUnmounted(() => {
  removeOpenTaskListener?.()
  removeOpenTaskListener = null
  if (trayStatusHeartbeat) clearInterval(trayStatusHeartbeat)
})
</script>

<template>
  <div class="app-layout">
    <AppSidebar />
    <main
      class="main-content"
      :class="mainContentClasses"
    >
      <RouterView v-slot="{ Component, route: currentRoute }">
        <KeepAlive>
          <component
            :is="Component"
            v-if="currentRoute.meta.keepAlive"
            :key="currentRoute.name ?? currentRoute.path"
          />
        </KeepAlive>
        <component
          :is="Component"
          v-if="!currentRoute.meta.keepAlive"
          :key="currentRoute.name ?? currentRoute.path"
        />
      </RouterView>
    </main>
  </div>
</template>

<style scoped>
.app-layout {
  display: flex;
  height: 100vh;
  overflow: hidden;
}

.main-content {
  flex: 1;
  min-width: 0;
  min-height: 0;
  height: 100vh;
}

.main-content-default {
  padding: 24px;
  background: #f5f6f8;
}

.main-content-custom {
  padding: 0;
  background: transparent;
}

.main-content-scroll {
  overflow-y: auto;
}

.main-content-no-scroll {
  overflow: hidden;
}

.main-content.api-content,
.main-content.agents-content,
.main-content.knowledge-content {
  overflow: hidden;
  padding: 0;
}

.main-content.api-content,
.main-content.knowledge-content {
  background: #fff;
}
</style>
