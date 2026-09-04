<template>
  <a-modal
    :open="open"
    :title="title"
    :closable="false"
    :mask-closable="false"
    :footer="null"
    @cancel="emit('close')"
  >
    <p class="stop-confirm-copy">{{ description }}</p>
    <a-checkbox
      v-model:checked="suppressFutureConfirm"
      class="stop-confirm-suppression"
      :disabled="loading"
    >
      不再提醒
    </a-checkbox>
    <div class="stop-confirm-actions">
      <a-button
        :disabled="loading"
        @click="emit('close')"
        >取消</a-button
      >
      <a-button
        type="primary"
        danger
        :loading="loading"
        @click="emit('confirm', suppressFutureConfirm)"
      >
        {{ confirmText }}
      </a-button>
    </div>
  </a-modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    loading?: boolean
    title?: string
    description?: string
    confirmText?: string
  }>(),
  {
    loading: false,
    title: '停止当前运行？',
    description: 'CLI 进程及其子进程将被停止，已经产生的对话和日志会保留。',
    confirmText: '停止运行',
  },
)

const emit = defineEmits<{
  close: []
  confirm: [suppressFutureConfirm: boolean]
}>()

const suppressFutureConfirm = ref(false)

watch(
  () => props.open,
  (open) => {
    if (open) suppressFutureConfirm.value = false
  },
)
</script>

<style scoped>
.stop-confirm-copy {
  margin: 0;
  color: #595959;
  font-size: 14px;
  line-height: 22px;
}

.stop-confirm-suppression {
  margin-top: 16px;
}

.stop-confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 24px;
}
</style>
