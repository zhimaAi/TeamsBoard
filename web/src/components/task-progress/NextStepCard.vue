<template>
  <div class="next-step-card">
    <div class="next-step-copy">
      <strong>{{ isLastStep ? t('workflows.task.progress.lastStepTitle') : t('workflows.task.progress.nextStepTitle', { step: nextStepName }) }}</strong>
      <span>{{
        isLastStep ? t('workflows.task.progress.lastStepDescription') : t('workflows.task.progress.nextStepDescription', { step: nextStepName })
      }}</span>
    </div>
    <button
      type="button"
      :disabled="disabled"
      @click="emit('confirm')"
    >
      <span>{{ completing ? t('workflows.task.progress.processing') : isLastStep ? t('workflows.task.progress.completeTask') : t('workflows.task.progress.enterNext') }}</span>
      <ArrowRightOutlined v-if="!completing" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { ArrowRightOutlined } from '@ant-design/icons-vue'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

withDefaults(
  defineProps<{
    nextStepName?: string
    isLastStep: boolean
    disabled?: boolean
    completing?: boolean
  }>(),
  { nextStepName: '', disabled: false, completing: false },
)

const emit = defineEmits<{
  confirm: []
}>()
</script>

<style scoped>
.next-step-card {
  display: flex;
  width: 470px;
  max-width: 100%;
  margin: 16px auto 0;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 12px 16px;
  border-radius: 12px;
  background: linear-gradient(90deg, #f0f2fd 0%, #dee4ff 100%);
}
.next-step-copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
}
.next-step-copy strong {
  color: #000;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}
.next-step-copy span {
  margin-top: 4px;
  color: #3a4559;
  font-size: 12px;
  line-height: 20px;
}
.next-step-card button {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  border: 0;
  border-radius: 32px;
  padding: 8px 12px;
  color: #fff;
  background: #3157e2;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.16);
  font-size: 16px;
  line-height: 24px;
  cursor: pointer;
}
.next-step-card button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
  box-shadow: none;
}
</style>
