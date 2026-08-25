<template>
  <article class="step-card">
    <div class="step-top">
      <span class="step-number">{{ index + 1 }}</span>
      <img
        :src="resolveAgentAvatar(step, index)"
        alt=""
      />
      <div class="step-copy">
        <strong>{{ step.name }}</strong>
        <span class="step-status">{{ isCloudPipeline ? '云端同步' : '本地配置' }}</span>
      </div>
      <div
        v-if="!isCloudPipeline"
        class="step-orders"
      >
        <button
          type="button"
          class="action-btn"
          aria-label="上移 Agent"
          :disabled="index === 0"
          @click="emit('move', -1)"
        >
          <ArrowUpOutlined />
        </button>
        <button
          type="button"
          class="action-btn"
          aria-label="下移 Agent"
          :disabled="index === total - 1"
          @click="emit('move', 1)"
        >
          <ArrowDownOutlined />
        </button>
        <button
          type="button"
          class="action-btn action-btn-danger"
          aria-label="移除 Agent"
          @click="emit('remove')"
        >
          <DeleteOutlined />
        </button>
      </div>
    </div>
    <p>{{ step.description || step.prompt || step.prompt_snapshot || '暂无描述' }}</p>
    <div
      v-if="!step.cli_type || !stepModel(step)"
      class="missing"
    >
      <ExclamationOutlined />未设置{{
        !step.cli_type && !stepModel(step)
          ? ' CLI 与模型'
          : !step.cli_type
            ? ' CLI'
            : '模型'
      }}，请点击“{{ isCloudPipeline ? '配置' : '编辑' }}”完成配置
    </div>
    <footer>
      <span
        v-if="step.cli_type"
        class="badge"
      >
        ⌘ {{ step.cli_type }}
      </span>
      <span
        v-if="stepModel(step)"
        class="badge model"
      >
        ▣ {{ stepModel(step) }}
      </span>
      <button
        type="button"
        class="step-card-action"
        @click="emit('edit')"
      >
        <EditOutlined />{{ isCloudPipeline ? '配置' : '编辑' }}
      </button>
    </footer>
  </article>
</template>

<script setup lang="ts">
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  DeleteOutlined,
  EditOutlined,
  ExclamationOutlined,
} from '@ant-design/icons-vue'
import type { PipelineStep } from '@/types/pipeline'
import { resolveAgentAvatar, stepModel } from './agentPipeline'

defineProps<{
  step: PipelineStep
  index: number
  total: number
  isCloudPipeline: boolean
}>()

const emit = defineEmits<{
  edit: []
  move: [offset: number]
  remove: []
}>()
</script>

<style scoped>
.step-card {
  padding: 14px;
  border: 1px solid #e3e7ed;
  border-radius: 10px;
  background: #fff;
  box-shadow: 0 2px 5px rgba(15, 23, 42, 0.025);
  transition: border-color 0.18s, background-color 0.18s, box-shadow 0.18s;
}

.step-card:hover {
  border-color: #0071e3;
  box-shadow: 0 1px 4px rgba(0, 113, 227, 0.16);
}

.step-top {
  display: flex;
  align-items: center;
  gap: 10px;
}

.step-number {
  display: flex;
  width: 20px;
  height: 20px;
  flex: 0 0 20px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: #1677ff;
  font-size: 10px;
}

.step-top > img {
  width: 32px;
  height: 32px;
  flex: 0 0 32px;
  border-radius: 50%;
  object-fit: cover;
}

.step-copy {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.step-top strong {
  color: #111827;
  font-size: 14px;
  font-weight: 500;
}

.step-status {
  color: #98a2b3;
  font-size: 11px;
}

.step-orders {
  display: flex;
  flex: 0 0 auto;
}

.action-btn {
  display: inline-flex;
  width: 24px;
  min-width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 6px;
  color: #a6b0bd;
  background: transparent;
  transition: color 0.18s, background-color 0.18s;
  cursor: pointer;
}

.action-btn:hover:not(:disabled) {
  color: rgba(0, 0, 0, 0.88);
  background: rgba(0, 0, 0, 0.06);
}

.action-btn:disabled {
  color: #d6dbe2;
  cursor: not-allowed;
}

.action-btn-danger:hover:not(:disabled) {
  color: #ff4d4f;
  background: #fff1f0;
}

.action-btn:focus-visible,
.step-card-action:focus-visible {
  outline: 2px solid #1677ff;
  outline-offset: 2px;
}

.step-card > p {
  display: -webkit-box;
  overflow: hidden;
  margin: 8px 0 0;
  color: #5f6b7a;
  font-size: 12px;
  line-height: 20px;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.missing {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-top: 7px;
  padding: 5px 8px;
  border-radius: 6px;
  color: #b45309;
  background: #fff7e6;
  font-size: 10px;
  font-weight: 500;
}

.step-card footer {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 9px;
}

.badge {
  padding: 2px 8px;
  border-radius: 6px;
  color: #667085;
  background: #f4f5f7;
  font-size: 10px;
  font-weight: 500;
  line-height: 16px;
}

.step-card-action {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
  padding: 3px 6px;
  border: 0;
  border-radius: 5px;
  color: #98a2b3;
  background: transparent;
  transition: color 0.18s, background-color 0.18s;
  cursor: pointer;
  font-size: 10px;
}

.step-card-action:hover {
  color: #1677ff;
  background: #f3f6fa;
}

@media (prefers-reduced-motion: reduce) {
  .step-card {
    transition: none;
  }
}
</style>
