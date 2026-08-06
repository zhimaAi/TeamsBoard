<script setup lang="ts">
import { ConfigProvider } from 'ant-design-vue'
import { themeConfig } from '@/config/theme'
</script>

<template>
  <ConfigProvider :theme="themeConfig">
    <RouterView />
  </ConfigProvider>
</template>
