<template>
  <nav
    class="step-strip scrollbar--subtle"
    aria-label="Agent 编排步骤"
  >
    <template
      v-for="(step, stepIndex) in steps"
      :key="step.uuid"
    >
      <span
        v-if="stepIndex > 0"
        class="step-link"
        :class="{ done: stepIndex <= currentStepIndex }"
        aria-hidden="true"
        ><ArrowRightOutlined
      /></span>
      <button
        type="button"
        class="step-card"
        :class="[`step-card--${stepState(step)}`, { selected: selectedStepUuid === step.uuid }]"
        :disabled="readOnly"
        @click="emit('select', step.uuid)"
      >
        <span
          class="step-avatar-wrap"
          :class="{ spinning: stepState(step) === 'process' }"
        >
          <span
            class="step-avatar has-image"
          >
            <img
              :src="resolveAgentAvatar(step, stepIndex)"
              alt=""
            />
          </span>
        </span>
        <span class="step-copy">{{ step.name }}</span>
        <span
          v-if="step.status === 'completed'"
          class="step-finish-marker"
          aria-hidden="true"
          ><CheckOutlined
        /></span>
      </button>
    </template>
  </nav>
</template>

<script setup lang="ts">
import { ArrowRightOutlined, CheckOutlined } from '@ant-design/icons-vue'
import { resolveAgentAvatar } from '@/config/agentAvatars'
import type { PipelineStep } from '@/types/pipeline'

const props = withDefaults(
  defineProps<{
    steps: PipelineStep[]
    currentStepIndex: number
    currentStepUuid: string
    selectedStepUuid: string
    readOnly?: boolean
  }>(),
  { readOnly: false },
)

const emit = defineEmits<{
  select: [uuid: string]
}>()

function stepState(step: PipelineStep) {
  if (step.status === 'completed') return 'finish'
  if (
    step.uuid === props.currentStepUuid &&
    step.execution_status &&
    step.execution_status !== 'idle'
  ) {
    return step.execution_status === 'failed' ? 'error' : 'process'
  }
  return 'wait'
}
</script>

<style scoped>
.step-strip {
  display: flex;
  gap: 12px;
  align-items: center;
  overflow-x: auto;
  overflow-y: hidden;
  padding: 0;
}
.step-card {
  position: relative;
  display: flex;
  box-sizing: border-box;
  width: 155px;
  height: 40px;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border: 1px solid transparent;
  border-radius: 6px;
  color: #262626;
  background: #f0f0f0;
  cursor: pointer;
  text-align: left;
  transition:
    background 0.15s,
    color 0.15s;
}
.step-card:not(:disabled):hover {
  border-color: var(--02, #262626);
  background: #e5e7eb;
}
.step-card.selected {
  color: #fff;
  background: #8c8c8c;
}
.step-card.selected:not(:disabled):hover {
  background: #262626;
}
.step-card:disabled {
  cursor: default;
  opacity: 1;
}
.step-card--finish .step-avatar {
  background: #22c55e;
}
.step-card--error .step-avatar {
  background: #ef4444;
}
.step-card--process .step-avatar {
  background: #3157e2;
}
.step-card.selected .step-avatar {
  background: #fff;
  color: #262626;
}
.step-card--wait .step-avatar {
  opacity: 0.4;
}
.step-card--wait .step-copy {
  opacity: 0.8;
}
.step-avatar-wrap {
  position: relative;
  display: flex;
  flex: 0 0 auto;
}
/* 进行中：头像外叠双圆环（底环+旋转弧），对应设计稿“进行中”步骤态 */
.step-avatar-wrap.spinning::before {
  content: '';
  position: absolute;
  inset: -3px;
  border: 1.5px solid #e4e6eb;
  border-radius: 50%;
}
.step-avatar-wrap.spinning::after {
  content: '';
  position: absolute;
  inset: -3px;
  border: 1.5px solid transparent;
  border-top-color: #8c8c8c;
  border-radius: 50%;
  animation: step-avatar-spin 1s linear infinite;
}
@keyframes step-avatar-spin {
  to {
    transform: rotate(360deg);
  }
}
.step-avatar {
  display: flex;
  overflow: hidden;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  color: #fff;
  background: #9ca3af;
  font-size: 11px;
  font-weight: 600;
}
.step-avatar.has-image {
  border: 1px solid #fff;
  box-sizing: border-box;
}
.step-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.step-copy {
  display: block;
  width: 86px;
  flex: 0 1 86px;
  overflow: hidden;
}
.step-copy strong {
  display: block;
  overflow: hidden;
  color: inherit;
  font-size: 14px;
  font-weight: 400;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.step-card.selected .step-copy strong {
  color: #fff;
  font-weight: 600;
}
.step-finish-marker {
  position: absolute;
  top: 0;
  left: 0;
  display: inline-flex;
  width: 12px;
  height: 12px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: #595959;
  font-size: 8px;
  line-height: 1;
}
.step-link {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 40px;
  color: #d9d9d9;
}
.step-link.done {
  color: #d9d9d9;
}
.step-link .anticon {
  font-size: 16px;
}
</style>
