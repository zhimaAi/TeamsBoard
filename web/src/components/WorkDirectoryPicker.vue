<template>
  <a-dropdown
    v-model:open="isOpen"
    :trigger="['click']"
    placement="bottomLeft"
    :get-popup-container="getPopupContainer"
    overlay-class-name="work-dir-dropdown"
    @open-change="handleOpenChange"
  >
    <slot :selected-dir="modelValue" :is-open="isOpen">
      <button
        type="button"
        class="work-dir-default-trigger"
        :class="{ 'is-selected': !!modelValue }"
      >
        <FolderOutlined />
        <span class="trigger-text">{{ displayLabel || t('workflows.task.create.chooseDirectory') }}</span>
      </button>
    </slot>

    <template #overlay>
      <div
        class="work-dir-panel"
        role="menu"
        @click.stop
      >
        <!-- 搜索输入框 -->
        <div class="panel-search">
          <SearchOutlined class="search-icon" />
          <input
            ref="searchInputRef"
            v-model="keyword"
            type="text"
            class="search-input"
            :placeholder="t('workflows.task.common.searchWorkDir') || '搜索工作目录'"
            @keydown.stop
          />
          <button
            v-if="keyword"
            type="button"
            class="clear-button"
            @click="keyword = ''"
          >
            <CloseCircleFilled />
          </button>
        </div>

        <!-- 目录列表 -->
        <div class="panel-list scrollbar--subtle">
          <div
            v-if="filteredItems.length === 0"
            class="empty-tip"
          >
            {{ t('workflows.task.common.noMatchingDirectories') || '暂无匹配的工作目录' }}
          </div>
          <button
            v-for="item in filteredItems"
            :key="item.path"
            type="button"
            class="dir-item"
            :class="{ 'is-selected': isSelected(item.path) }"
            :title="item.path"
            @click="handleSelect(item.path)"
          >
            <div class="dir-item__main">
              <FolderOutlined class="item-icon" />
              <div class="dir-item__text">
                <span
                  v-if="item.projectName"
                  class="item-title"
                >{{ item.projectName }}</span>
                <span
                  class="item-path"
                  :class="{ 'item-path--single': !item.projectName }"
                >{{ item.path }}</span>
              </div>
            </div>
            <CheckOutlined
              v-if="isSelected(item.path)"
              class="check-icon"
            />
          </button>
        </div>

        <!-- 底部：打开本地文件夹 -->
        <div class="panel-footer">
          <div
            v-if="!isDesktopRuntime()"
            class="manual-directory-row"
          >
            <input
              v-model="manualPath"
              type="text"
              class="manual-directory-input"
              :placeholder="t('components.directory.manualPlaceholder')"
              @keydown.enter.prevent="handleManualSelect"
            />
            <button
              type="button"
              class="manual-directory-btn"
              :disabled="!manualPath.trim()"
              @click="handleManualSelect"
            >
              {{ t('components.directory.useManual') }}
            </button>
          </div>
          <button
            type="button"
            class="open-folder-btn"
            :disabled="openingFolder || !isDesktopRuntime()"
            @click="handleOpenLocalFolder"
          >
            <FolderAddOutlined class="btn-icon" />
            <span>{{ t('workflows.task.common.openLocalFolder') || '打开本地文件夹' }}</span>
          </button>
        </div>
      </div>
    </template>
  </a-dropdown>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  FolderOutlined,
  FolderAddOutlined,
  SearchOutlined,
  CheckOutlined,
  CloseCircleFilled,
} from '@ant-design/icons-vue'
import { useConversationStore, getLastDirSegment } from '@/stores/conversation'
import { isDesktopRuntime, selectDirectory } from '@/composables/useDesktop'
import { useAppI18n } from '@/i18n'

interface Props {
  modelValue?: string
  getPopupContainer?: (triggerNode: HTMLElement) => HTMLElement
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: '',
  getPopupContainer: undefined,
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'select', value: string): void
}>()

const { t } = useAppI18n()
const conversationStore = useConversationStore()

const isOpen = ref(false)
const keyword = ref('')
const openingFolder = ref(false)
const manualPath = ref('')
const searchInputRef = ref<HTMLInputElement | null>(null)

interface DirItem {
  path: string
  projectName?: string
}

const allDirectories = computed<DirItem[]>(() => {
  const dirs = conversationStore.allKnownWorkDirs
  const projects = conversationStore.projects

  const norm = (p: string) => (p || '').replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase()

  return dirs.map((d) => {
    const matched = projects.find((p) => p.local_dir && norm(p.local_dir) === norm(d))
    return {
      path: d,
      projectName: matched ? matched.name : undefined,
    }
  })
})

const filteredItems = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  if (!q) return allDirectories.value
  return allDirectories.value.filter(
    (item) =>
      item.path.toLowerCase().includes(q) ||
      (item.projectName && item.projectName.toLowerCase().includes(q)),
  )
})

const displayLabel = computed(() => {
  if (!props.modelValue) return ''
  const info = conversationStore.resolveWorkDirInfo(props.modelValue)
  return info.projectName || getLastDirSegment(props.modelValue) || props.modelValue
})

function isSelected(path: string): boolean {
  if (!props.modelValue) return false
  const norm = (p: string) => (p || '').replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase()
  return norm(path) === norm(props.modelValue)
}

function handleSelect(path: string) {
  conversationStore.recordWorkingDir(path)
  emit('update:modelValue', path)
  emit('select', path)
  isOpen.value = false
}

async function handleOpenLocalFolder() {
  if (openingFolder.value || !isDesktopRuntime()) return
  openingFolder.value = true
  try {
    const selected = await selectDirectory(props.modelValue)
    if (selected) {
      handleSelect(selected)
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('components.errors.selectDirectoryUnsupported'))
  } finally {
    openingFolder.value = false
  }
}

function handleManualSelect() {
  const path = manualPath.value.trim()
  if (!path) return
  handleSelect(path)
  manualPath.value = ''
}

function handleOpenChange(open: boolean) {
  isOpen.value = open
  if (open) {
    keyword.value = ''
    void conversationStore.loadProjects()
    nextTick(() => {
      searchInputRef.value?.focus()
    })
  }
}
</script>

<style scoped>
.work-dir-default-trigger {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #f9f9f9;
  color: #595959;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s;
}

.work-dir-default-trigger:hover {
  background: #f0f0f0;
  border-color: #d1d5db;
}

.work-dir-default-trigger.is-selected {
  background: #f0f7ff;
  border-color: #bfdbfe;
  color: #2563eb;
}

.trigger-text {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.work-dir-panel {
  width: 420px;
  max-width: calc(100vw - 32px);
  max-height: 440px;
  background: #ffffff;
  border-radius: 12px;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.1);
  border: 1px solid #f0f0f0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.panel-search {
  position: relative;
  margin: 10px 10px 6px;
  background: #f5f5f5;
  border-radius: 8px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  height: 34px;
}

.search-icon {
  color: #8b8e94;
  font-size: 14px;
  margin-right: 8px;
}

.search-input {
  flex: 1;
  border: none;
  background: transparent;
  outline: none;
  font-size: 13px;
  color: #262626;
}

.search-input::placeholder {
  color: #bfbfbf;
}

.clear-button {
  background: none;
  border: none;
  color: #bfbfbf;
  cursor: pointer;
  padding: 2px;
  display: flex;
  align-items: center;
}

.clear-button:hover {
  color: #8c8c8c;
}

.panel-list {
  flex: 1;
  overflow-y: auto;
  padding: 4px 8px;
  max-height: 300px;
}

.empty-tip {
  padding: 24px 0;
  text-align: center;
  color: #8c8c8c;
  font-size: 13px;
}

.dir-item {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border: none;
  border-radius: 6px;
  background: transparent;
  cursor: pointer;
  text-align: left;
  transition: background 0.15s;
  margin-bottom: 2px;
}

.dir-item:hover {
  background: #f5f9ff;
}

.dir-item.is-selected {
  background: #f5f9ff;
}

.dir-item__main {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex: 1;
}

.item-icon {
  font-size: 16px;
  color: #595959;
  flex-shrink: 0;
}

.dir-item__text {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
}

.item-title {
  font-size: 14px;
  font-weight: 500;
  color: #262626;
  line-height: 20px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.item-path {
  font-size: 12px;
  color: #8c8c8c;
  line-height: 18px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.item-path--single {
  font-size: 13px;
  color: #262626;
}

.check-icon {
  color: #3157e2;
  font-size: 14px;
  margin-left: 8px;
  flex-shrink: 0;
}

.panel-footer {
  padding: 8px 10px;
  border-top: 1px solid #f0f0f0;
  background: #ffffff;
}

.manual-directory-row {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}

.manual-directory-input {
  flex: 1;
  min-width: 0;
  height: 32px;
  padding: 0 8px;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  outline: none;
  color: #262626;
  font-size: 12px;
}

.manual-directory-input:focus {
  border-color: #3157e2;
  box-shadow: 0 0 0 2px rgba(49, 87, 226, 0.15);
}

.manual-directory-btn {
  flex: 0 0 auto;
  height: 32px;
  padding: 0 8px;
  border: 0;
  border-radius: 6px;
  background: #3157e2;
  color: #fff;
  cursor: pointer;
  font-size: 12px;
}

.manual-directory-btn:disabled {
  background: #d9d9d9;
  cursor: not-allowed;
}

.open-folder-btn {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: none;
  border-radius: 6px;
  background: #f0f0f0;
  color: #44474d;
  font-size: 14px;
  cursor: pointer;
  transition: background 0.2s;
}

.open-folder-btn:hover {
  background: #e6e6e6;
  color: #262626;
}

.btn-icon {
  font-size: 16px;
}
</style>
