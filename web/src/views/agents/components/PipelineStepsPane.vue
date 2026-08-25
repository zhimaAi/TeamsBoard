<template>
  <section class="step-pane">
    <header class="pane-header">
      <h3>
        <RobotOutlined /> Agent 编排
        <span v-if="pipeline" class="pipeline-name">— {{ pipeline.name }}</span>
      </h3>
      <div
        v-if="pipeline && !isCloudPipeline"
        class="add-wrap"
      >
        <a-button
          type="primary"
          size="small"
          @click="addMenuOpen = !addMenuOpen"
        >
          <template #icon><PlusOutlined /></template>
          添加
        </a-button>
        <div
          v-if="addMenuOpen"
          class="add-menu"
        >
          <button @click="openAgentEditor"><PlusOutlined />新建 Agent</button>
          <button @click="openCopyModal"><RobotOutlined />从已添加 Agent 选择</button>
        </div>
      </div>
    </header>
    <div class="step-list">
      <template
        v-for="(step, index) in sortedSteps"
        :key="step.uuid"
      >
        <div
          v-if="index"
          class="flow-arrow"
        >
          <span class="flow-arrow-line" />
          <ArrowDownOutlined aria-hidden="true" />
          <span class="flow-arrow-line" />
        </div>
        <PipelineStepCard
          :step="step"
          :index="index"
          :total="sortedSteps.length"
          :is-cloud-pipeline="isCloudPipeline"
          @edit="emit('edit-agent', step)"
          @move="emit('move-agent', step, $event)"
          @remove="emit('remove-agent', step)"
        />
      </template>
      <a-empty
        v-if="pipeline && !sortedSteps.length"
        description="当前流水线暂无 Agent，点击上方“添加”开始编排"
      />
      <a-empty
        v-else-if="!pipeline"
        description="请选择左侧流水线"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  ArrowDownOutlined,
  PlusOutlined,
  RobotOutlined,
} from '@ant-design/icons-vue'
import type { Pipeline, PipelineStep } from '@/types/pipeline'
import { isPipelineCloud as isCloudPipelineSource, sortPipelineSteps } from './agentPipeline'
import PipelineStepCard from './PipelineStepCard.vue'

const props = defineProps<{
  pipeline?: Pipeline
}>()

const emit = defineEmits<{
  'create-agent': []
  'copy-agent': []
  'edit-agent': [step: PipelineStep]
  'move-agent': [step: PipelineStep, offset: number]
  'remove-agent': [step: PipelineStep]
}>()

const addMenuOpen = ref(false)
const isCloudPipeline = computed(() => isCloudPipelineSource(props.pipeline))
const sortedSteps = computed(() => sortPipelineSteps(props.pipeline?.steps || []))

function openAgentEditor() {
  addMenuOpen.value = false
  emit('create-agent')
}

function openCopyModal() {
  addMenuOpen.value = false
  emit('copy-agent')
}
</script>

<style scoped>
.step-pane {
  display: flex;
  height: 100%;
  min-width: 0;
  min-height: 0;
  flex: 1;
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

.pane-header h3 .pipeline-name {
  color: #98a2b3;
  font-size: 12px;
  font-weight: 400;
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

.step-list {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  scrollbar-color: #d7dce3 transparent;
  scrollbar-width: thin;
}

.add-wrap {
  position: relative;
}

.add-menu {
  position: absolute;
  z-index: 20;
  top: 38px;
  right: 0;
  width: 240px;
  padding: 8px;
  border: 1px solid #e3e7ed;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 12px 30px rgba(15, 23, 42, 0.14);
}

.add-menu button {
  display: flex;
  width: 100%;
  height: 42px;
  align-items: center;
  gap: 10px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  color: #344054;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  text-align: left;
  white-space: nowrap;
}

.add-menu button:hover {
  color: #111827;
  background: #f4f7fb;
}

.add-menu button :deep(svg) {
  color: #1677ff;
  font-size: 15px;
}

.add-menu button:focus-visible {
  outline: 2px solid #1677ff;
  outline-offset: 2px;
}

.flow-arrow {
  display: flex;
  height: 34px;
  align-items: center;
  justify-content: center;
  gap: 6px;
  color: #9aa6b5;
  font-size: 14px;
  line-height: 1;
}

.flow-arrow-line {
  width: 40px;
  height: 1px;
  background: #e3e7ed;
}

@media (max-width: 900px) {
  .step-pane {
    height: auto;
    min-height: 420px;
  }
}
</style>
