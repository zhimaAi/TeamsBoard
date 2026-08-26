<template>
  <section class="pipeline-pane">
    <header class="pane-header">
      <h3>⌘ 流水线</h3>
      <a-button
        type="primary"
        size="small"
        class="create-button"
        @click="emit('create')"
      >
        <template #icon><PlusOutlined /></template>
        <span>新建</span>
      </a-button>
    </header>
    <div class="pipeline-list">
      <article
        v-for="pipeline in pipelines"
        :key="pipeline.uuid"
        class="pipeline-card"
        :class="{ active: selectedUuid === pipeline.uuid }"
      >
        <div
          class="pipeline-main"
          role="button"
          tabindex="0"
          :aria-pressed="selectedUuid === pipeline.uuid"
          @click="emit('select', pipeline.uuid)"
          @keydown.enter="emit('select', pipeline.uuid)"
          @keydown.space.prevent="emit('select', pipeline.uuid)"
        >
          <div class="pipeline-summary">
            <img
              :src="pipeline.avatar || PIPELINE_AVATARS[0]"
              alt=""
            />
            <div class="pipeline-copy">
              <span class="pipeline-title">
                <strong>{{ pipeline.name }}</strong>
                <em :class="isPipelineCloud(pipeline) ? 'cloud' : 'local'">
                  {{ isPipelineCloud(pipeline) ? '团队' : '私有' }}
                </em>
                <a-tooltip v-if="missingSteps(pipeline).length">
                  <template #title>
                    <b>有 Agent 未配置 CLI 或模型</b><br />请先完成配置
                  </template>
                  <span class="abnormal">
                    <ExclamationOutlined />
                  </span>
                </a-tooltip>
              </span>
              <div class="pipeline-description">{{ pipeline.description || '暂无简介' }}</div>
            </div>
          </div>
          <span class="avatar-chain">
            <span
              v-if="isPipelineCloud(pipeline)"
              class="avatar-chain-copy"
            >
              团队流水线，由团队成员协作完成
            </span>
            <div class="avatar-chain-list" v-else>
              <template
                v-for="(step, index) in pipeline.steps || []"
                :key="step.uuid"
              >
                <b v-if="index">→</b>
                <a-tooltip :title="step.name">
                  <img
                    :src="resolveAgentAvatar(step, index)"
                    alt=""
                  />
                </a-tooltip>
              </template>
            </div>
          </span>
        </div>
        <button
          v-if="!isPipelineCloud(pipeline)"
          type="button"
          class="action-btn pipeline-edit"
          aria-label="编辑流水线"
          @click.stop="emit('edit', pipeline)"
        >
          <EditOutlined />
        </button>
      </article>
      <a-empty
        v-if="!pipelines.length"
        description="暂无流水线，点击上方“新建”创建"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { EditOutlined, ExclamationOutlined, PlusOutlined } from '@ant-design/icons-vue'
import type { Pipeline } from '@/types/pipeline'
import {
  isPipelineCloud,
  missingSteps,
  PIPELINE_AVATARS,
  resolveAgentAvatar,
} from './agentPipeline'

defineProps<{
  pipelines: Pipeline[]
  selectedUuid: string
}>()

const emit = defineEmits<{
  create: []
  edit: [pipeline: Pipeline]
  select: [uuid: string]
}>()
</script>

<style scoped>
.pipeline-pane {
  display: flex;
  width: 392px;
  height: 100%;
  min-width: 0;
  min-height: 0;
  flex: 0 0 392px;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid #e3e7ed;
  border-radius: 16px;
  background: #fff;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.06);
}

.pane-header {
  display: flex;
  height: 54px;
  flex: 0 0 54px;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  border-bottom: 1px solid #eef0f3;
}

.pane-header h3 {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: #111827;
  font-size: 14px;
  font-weight: 600;
}

.create-button {
  font-size: 13px;
}

.pane-header :deep(.ant-btn) {
  height: 28px;
  padding-inline: 14px;
  border-radius: 14px;
  box-shadow: none;
}

.pane-header :deep(.ant-btn-primary) {
  border-color: rgb(0, 113, 227);
  background: rgb(0, 113, 227);
}

.pane-header :deep(.ant-btn-primary:hover) {
  border-color: rgb(0, 119, 237);
  background: rgb(0, 119, 237);
}

.pane-header :deep(.ant-btn-primary:active) {
  border-color: rgb(0, 104, 209);
  background: rgb(0, 104, 209);
}

.pipeline-list {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 12px;
  overflow-y: auto;
  padding: 16px;
  scrollbar-color: #d7dce3 transparent;
  scrollbar-width: thin;
}

.pipeline-card {
  position: relative;
  width: 100%;
  border: 1px solid #e3e7ed;
  border-radius: 10px;
  background: #fff;
  transition: border-color 0.18s, background-color 0.18s, box-shadow 0.18s;
}

.pipeline-card:hover {
  border-color: #cbd3dd;
  box-shadow: 0 3px 10px rgba(15, 23, 42, 0.06);
}

.pipeline-card.active {
  border-color: #78b7ff;
  background: #f0f7ff;
  box-shadow: 0 2px 8px rgba(22, 119, 255, 0.1);
}

.pipeline-main {
  display: flex;
  width: 100%;
  min-height: 132px;
  box-sizing: border-box;
  flex-direction: column;
  padding: 14px;
  border: 0;
  border-radius: inherit;
  color: #5f6b7a;
  background: transparent;
  cursor: pointer;
  font: inherit;
  text-align: left;
}

.pipeline-main:focus-visible,
.action-btn:focus-visible {
  outline: 2px solid #1677ff;
  outline-offset: 2px;
}

.pipeline-summary {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 12px;
}

.pipeline-summary > img {
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  border-radius: 50%;
  object-fit: cover;
}

.pipeline-copy {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}

.pipeline-title {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  padding-right: 32px;
}

.pipeline-title strong {
  overflow: hidden;
  color: #111827;
  font-size: 14px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pipeline-title em {
  display: inline-flex;
  height: 22px;
  flex: 0 0 auto;
  align-items: center;
  padding: 0 8px;
  border-radius: 999px;
  color: #1677ff;
  background: #e6f4ff;
  font-size: 12px;
  font-style: normal;
  line-height: 1;
}

.pipeline-title em.cloud {
  color: #fa8c16;
  background: #fff7e6;
}

.abnormal {
  display: flex;
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #ef4444;
  background: #fef2f2;
  font-size: 9px;
}

.pipeline-description {
  display: -webkit-box;
  overflow: hidden;
  margin-top: 4px;
  color: #5f6b7a;
  font-size: 12px;
  line-height: 18px;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.avatar-chain {
  display: flex;
  width: 100%;
  min-height: 44px;
  box-sizing: border-box;
  align-items: center;
  gap: 4px;
  margin-top: 10px;
  padding: 7px 10px;
  border-radius: 8px;
  background-color: rgb(0 0 0 / 0.05);
  overflow: hidden;
  .avatar-chain-list{
    width: 100%;
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: nowrap;
    overflow: hidden;
  }
}

.avatar-chain img {
  width: 28px;
  height: 28px;
  border: 2px solid #fff;
  border-radius: 50%;
  object-fit: cover;
}

.avatar-chain b {
  color: #c1c8d1;
  font-size: 10px;
}

.avatar-chain-copy {
  color: #5f6b7a;
  font-size: 12px;
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

.action-btn:hover {
  color: rgba(0, 0, 0, 0.88);
  background: rgba(0, 0, 0, 0.06);
}

.pipeline-edit {
  position: absolute;
  z-index: 1;
  top: 8px;
  right: 8px;
}

@media (max-width: 900px) {
  .pipeline-pane {
    width: 100%;
    height: auto;
    min-height: 340px;
    flex: 0 0 340px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .pipeline-card {
    transition: none;
  }
}
</style>
