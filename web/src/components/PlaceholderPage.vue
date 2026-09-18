<script setup lang="ts">
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

defineProps<{
  title: string
  description?: string
}>()
</script>

<template>
  <div class="placeholder-page">
    <a-card :title="title">
      <a-empty :description="description || t('components.placeholder.developing', { title })" />
    </a-card>
  </div>
</template>

<style scoped>
.placeholder-page {
  max-width: 1200px;
  margin: 0 auto;
}
</style>
