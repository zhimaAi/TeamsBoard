<script setup lang="ts">
defineProps<{
  title: string
  description?: string
}>()
</script>

<template>
  <div class="placeholder-page">
    <a-card :title="title">
      <a-empty :description="description || `${title}功能开发中`" />
    </a-card>
  </div>
</template>

<style scoped>
.placeholder-page {
  max-width: 1200px;
  margin: 0 auto;
}
</style>
