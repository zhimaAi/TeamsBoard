<template>
  <a-modal
    :open="open"
    :footer="null"
    width="min(90vw, 900px)"
    centered
    destroy-on-close
    @update:open="handleOpenChange"
  >
    <img :src="imageUrl" class="preview-image" alt="" />
  </a-modal>
</template>

<script setup lang="ts">
defineProps<{
  open: boolean
  imageUrl: string
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
}>()

function handleOpenChange(open: boolean) {
  emit('update:open', open)
}
</script>

<style scoped>
.preview-image {
  display: block;
  width: 100%;
  height: auto;
}
</style>
