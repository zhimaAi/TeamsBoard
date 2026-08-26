<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { syncTrayTaskCount } from '@/composables/useDesktop'
import { useLocalWS } from '@/composables/useLocalWebSocket'
import { provideDocumentTitle } from '@/composables/useDocumentTitle'
import AppSidebar from './components/AppSidebar.vue'

const route = useRoute()
const appStore = useAppStore()

const currentMenuTitle = computed(() => (
  typeof route.meta.title === 'string' ? route.meta.title : undefined
))
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
  appStore.setUnreadTaskNotifications(data?.count ?? 0)
})

watch(() => appStore.cliTaskCount, syncTrayTaskCount, { immediate: true })

let trayStatusHeartbeat: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  trayStatusHeartbeat = setInterval(() => {
    syncTrayTaskCount(appStore.cliTaskCount)
  }, 20_000)
})
onUnmounted(() => {
  if (trayStatusHeartbeat) clearInterval(trayStatusHeartbeat)
})
</script>

<template>
  <div class="app-layout">
    <AppSidebar />
    <main class="main-content" :class="mainContentClasses">
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
