<template>
  <div v-if="open" class="document-mention-menu" role="listbox" aria-label="选择 Agent 产出文档">
    <div class="mention-title">
      <span>选择 Agent 产出文档</span>
      <span v-if="query">匹配“{{ query }}”</span>
    </div>
    <div v-if="loading" class="mention-state"><a-spin size="small" />正在读取文档…</div>
    <div v-else-if="error" class="mention-state mention-error">
      <span>{{ error }}</span>
      <button type="button" @mousedown.prevent="loadFiles">重试</button>
    </div>
    <div v-else-if="!filteredFiles.length" class="mention-state">暂无匹配的 Agent 产出文档</div>
    <template v-else>
      <button
        v-for="(file, index) in filteredFiles"
        :key="file.absolute_path"
        type="button"
        class="mention-item"
        :class="{ active: index === activeIndex }"
        role="option"
        :aria-selected="index === activeIndex"
        @mouseenter="activeIndex = index"
        @mousedown.prevent="selectFile(file)"
      >
        <span class="agent-badge">{{ initials(file.owner_step_name) }}</span>
        <span class="mention-copy">
          <strong>{{ file.name }}</strong>
          <span>{{ file.owner_step_name || 'Agent' }}</span>
        </span>
        <span class="mention-type">{{ file.file_type || '文件' }}</span>
      </button>
    </template>
    <div v-if="!loading && !error && filteredFiles.length" class="mention-footer">↑↓ 选择 · Enter 插入 · Esc 关闭</div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import apiClient from '@/api/client'
import type { TaskFileNode, TaskFilesResponse } from '@/types/task-files'
import { initials } from './utils'

interface MentionFile extends TaskFileNode {
  absolute_path: string
  owner_step_uuid: string
}

const props = defineProps<{
  open: boolean
  taskUuid: string
  query: string
}>()

const emit = defineEmits<{
  close: []
  select: [file: MentionFile]
}>()

const loading = ref(false)
const error = ref('')
const files = ref<MentionFile[]>([])
const activeIndex = ref(0)
let loadRequestId = 0

const filteredFiles = computed(() => {
  const keyword = props.query.trim().toLocaleLowerCase()
  const result = keyword
    ? files.value.filter(file => (
      file.name.toLocaleLowerCase().includes(keyword)
      || (file.owner_step_name || '').toLocaleLowerCase().includes(keyword)
      || file.path.toLocaleLowerCase().includes(keyword)
    ))
    : files.value
  return result.slice(0, 50)
})

watch(() => props.open, (open) => {
  if (open) void loadFiles()
})

watch(() => props.query, () => {
  activeIndex.value = 0
})

watch(() => filteredFiles.value.length, (length) => {
  if (!length) activeIndex.value = 0
  else if (activeIndex.value >= length) activeIndex.value = length - 1
})

async function loadFiles() {
  if (!props.taskUuid) return
  const requestId = ++loadRequestId
  loading.value = true
  error.value = ''
  try {
    const response = await apiClient.get<TaskFilesResponse>(
      `/tasks/${encodeURIComponent(props.taskUuid)}/files`,
    )
    if (requestId !== loadRequestId) return
    files.value = flattenAgentFiles(response.tree || [])
    activeIndex.value = 0
  } catch (loadError) {
    if (requestId !== loadRequestId) return
    error.value = loadError instanceof Error ? loadError.message : '文档列表加载失败'
  } finally {
    if (requestId === loadRequestId) loading.value = false
  }
}

function flattenAgentFiles(nodes: TaskFileNode[]): MentionFile[] {
  const result: MentionFile[] = []
  for (const node of nodes) {
    if (node.is_dir) {
      result.push(...flattenAgentFiles(node.children || []))
      continue
    }
    if (node.absolute_path && node.owner_step_uuid) {
      result.push(node as MentionFile)
    }
  }
  return result
}

function selectFile(file: MentionFile) {
  emit('select', file)
}

function handleKeydown(event: KeyboardEvent) {
  if (!props.open) return false
  if (event.key === 'Escape') {
    event.preventDefault()
    emit('close')
    return true
  }
  if (!filteredFiles.value.length) {
    if (['ArrowDown', 'ArrowUp', 'Enter', 'Tab'].includes(event.key)) {
      event.preventDefault()
      return true
    }
    return false
  }
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    activeIndex.value = (activeIndex.value + 1) % filteredFiles.value.length
    return true
  }
  if (event.key === 'ArrowUp') {
    event.preventDefault()
    activeIndex.value = (activeIndex.value - 1 + filteredFiles.value.length) % filteredFiles.value.length
    return true
  }
  if (event.key === 'Enter' || event.key === 'Tab') {
    event.preventDefault()
    const file = filteredFiles.value[activeIndex.value]
    if (file) selectFile(file)
    return true
  }
  return false
}

defineExpose({ handleKeydown })
</script>

<style scoped>
.document-mention-menu {
  position: absolute;
  z-index: 30;
  right: 12px;
  bottom: 48px;
  left: 12px;
  max-height: 310px;
  overflow-y: auto;
  border: 1px solid #d9e2ef;
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 14px 36px rgba(15, 23, 42, 0.18);
}
.mention-title,
.mention-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  color: #98a2b3;
  font-size: 11px;
}
.mention-title {
  position: sticky;
  z-index: 1;
  top: 0;
  border-bottom: 1px solid #eef2f6;
  background: #fff;
}
.mention-title span:first-child {
  color: #475467;
  font-weight: 600;
}
.mention-state {
  display: flex;
  min-height: 54px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 12px;
  color: #98a2b3;
  font-size: 12px;
}
.mention-error {
  color: #d4380d;
}
.mention-error button {
  border: 0;
  color: #1677ff;
  background: transparent;
  cursor: pointer;
}
.mention-item {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border: 0;
  color: #344054;
  background: #fff;
  cursor: pointer;
  text-align: left;
}
.mention-item:hover,
.mention-item.active {
  background: #f0f6ff;
}
.agent-badge {
  display: inline-flex;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #3157e2;
  background: #eaf0ff;
  font-size: 11px;
  font-weight: 600;
}
.mention-copy {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}
.mention-copy strong,
.mention-copy span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mention-copy strong {
  color: #1d2939;
  font-size: 13px;
  font-weight: 500;
}
.mention-copy span {
  color: #98a2b3;
  font-size: 11px;
}
.mention-type {
  flex: 0 0 auto;
  padding: 2px 6px;
  border-radius: 4px;
  color: #667085;
  background: #f2f4f7;
  font-size: 10px;
}
.mention-footer {
  position: sticky;
  bottom: 0;
  justify-content: flex-end;
  border-top: 1px solid #eef2f6;
  background: #fff;
}
</style>
