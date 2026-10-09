<script setup lang="ts">
import { ConfigProvider } from 'ant-design-vue'
import { defineAsyncComponent, onMounted, onUnmounted } from 'vue'
import { themeConfig } from '@/config/theme'
import { stopLocaleSync } from '@/i18n'
import { useUpdateStore } from '@/stores/update'
import { useLocale } from '@/composables/useLocale'

const { antLocale } = useLocale()
const updateStore = useUpdateStore()
const DesktopUpdateModal = defineAsyncComponent(() => import('@/components/DesktopUpdateModal.vue'))

// 检查更新不影响登录和工作区启动；桌面主进程负责缓存和请求。
onMounted(() => { void updateStore.check(false).catch(() => {}) })

onUnmounted(() => { stopLocaleSync(); updateStore.dispose() })
</script>

<template>
  <ConfigProvider :locale="antLocale" :theme="themeConfig">
    <RouterView />
    <DesktopUpdateModal v-if="updateStore.modalOpen" />
  </ConfigProvider>
</template>
