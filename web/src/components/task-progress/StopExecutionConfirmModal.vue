<template>
  <a-modal
    :open="open"
    :title="resolvedTitle"
    :closable="false"
    :mask-closable="false"
    :footer="null"
    @cancel="emit('close')"
  >
    <p class="stop-confirm-copy">{{ resolvedDescription }}</p>
    <a-checkbox
      v-model:checked="suppressFutureConfirm"
      class="stop-confirm-suppression"
      :disabled="loading"
    >
      {{ t('workflows.task.progress.dontRemind') }}
    </a-checkbox>
    <div class="stop-confirm-actions">
      <a-button
        :disabled="loading"
        @click="emit('close')"
        >{{ t('common.actions.cancel') }}</a-button
      >
      <a-button
        type="primary"
        danger
        :loading="loading"
        @click="emit('confirm', suppressFutureConfirm)"
      >
        {{ resolvedConfirmText }}
      </a-button>
    </div>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

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
  },
)

const resolvedTitle = computed(() => props.title ?? t('workflows.task.progress.stopTitle'))
const resolvedDescription = computed(() =>
  props.description ?? t('workflows.task.progress.stopDescription'),
)
const resolvedConfirmText = computed(() =>
  props.confirmText ?? t('workflows.task.progress.stopConfirm'),
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
