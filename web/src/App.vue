<script setup lang="ts">
import { ConfigProvider } from 'ant-design-vue'
import { onUnmounted } from 'vue'
import { themeConfig } from '@/config/theme'
import { stopLocaleSync } from '@/i18n'
import { useLocale } from '@/composables/useLocale'

const { antLocale } = useLocale()

onUnmounted(stopLocaleSync)
</script>

<template>
  <ConfigProvider :locale="antLocale" :theme="themeConfig">
    <RouterView />
  </ConfigProvider>
</template>
