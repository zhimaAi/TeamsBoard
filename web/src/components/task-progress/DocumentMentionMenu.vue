<template>
  <Transition name="mention-pop">
    <div
      v-if="open"
      ref="menuRef"
      class="document-mention-menu"
      role="listbox"
      :aria-label="t('workflows.task.progress.chooseDocuments')"
    >
      <div class="mention-title">
        <span class="mention-title-label">{{ t('workflows.task.progress.chooseDocuments') }}</span>
        <span v-if="normalizedQuery" class="mention-title-meta">
          {{ t('workflows.task.progress.queryMatch', { query: normalizedQuery }) }}
        </span>
        <span v-else-if="status === 'ready'" class="mention-title-meta">
          {{ t('workflows.task.progress.mentionFileCount', { count: items.length }) }}
        </span>
      </div>
      <div v-if="status === 'loading'" class="mention-state">
        <a-spin size="small" />
        <span>{{ t('workflows.task.progress.readingDocuments') }}</span>
      </div>
      <div v-else-if="status === 'error'" class="mention-state mention-error">
        <span>{{ error || t('workflows.task.progress.documentsLoadFailed') }}</span>
        <button type="button" @mousedown.prevent="emit('retry')">
          {{ t('common.actions.retry') }}
        </button>
      </div>
      <template v-else>
        <button
          v-for="(file, index) in items"
          :key="file.absolute_path"
          :ref="(element) => setItemRef(element, index)"
          type="button"
          class="mention-item"
          :class="{ active: index === activeIndex }"
          role="option"
          :aria-selected="index === activeIndex"
          @mouseenter="activeIndex = index"
          @mousedown.prevent="selectFile(file)"
        >
          <span class="agent-badge">{{ initials(sourceLabel(file)) }}</span>
          <span class="mention-copy">
            <strong class="mention-name">
              <span
                v-for="(segment, segmentIndex) in splitDocumentMentionHighlight(file.name, file.nameRanges)"
                :key="segmentIndex"
                :class="{ 'mention-hit': segment.matched }"
              >{{ segment.text }}</span>
            </strong>
            <span class="mention-owner">{{ sourceLabel(file) }}</span>
          </span>
          <span class="mention-type">{{ file.file_type || t('workflows.task.progress.file') }}</span>
        </button>
      </template>
      <div v-if="status === 'ready' && items.length" class="mention-footer">
        {{ t('workflows.task.progress.keyboardHint') }}
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed, ref, watch, type ComponentPublicInstance } from 'vue'
import { initials } from './utils'
import { splitDocumentMentionHighlight } from './documentMention'
import type { DocumentMentionCandidate } from './documentMention'
import type { TaskFilesIndexStatus } from '@/composables/useTaskFilesIndex'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

const props = defineProps<{
  open: boolean
  query: string
  items: DocumentMentionCandidate[]
  status: TaskFilesIndexStatus
  error: string
}>()

const emit = defineEmits<{
  close: []
  select: [file: DocumentMentionCandidate]
  retry: []
}>()

const menuRef = ref<HTMLElement>()
const itemRefs = ref<HTMLElement[]>([])
const activeIndex = ref(0)

const normalizedQuery = computed(() => props.query.trim())

watch(
  () => props.items,
  () => {
    // 候选集合每次都是重新计算的新数组，重置选中项与 DOM 引用，避免指向已移除的条目
    activeIndex.value = 0
    itemRefs.value = []
  },
)

watch(normalizedQuery, () => {
  activeIndex.value = 0
})

watch(
  () => props.items.length,
  (length) => {
    if (!length) {
      activeIndex.value = 0
      return
    }
    if (activeIndex.value >= length) activeIndex.value = length - 1
  },
)

watch(activeIndex, (index) => {
  const menu = menuRef.value
  const item = itemRefs.value[index]
  if (!menu || !item) return
  // 只滚动菜单自身，不用 scrollIntoView，避免连带滚动外层会话区域
  const itemTop = item.offsetTop
  const itemBottom = itemTop + item.offsetHeight
  if (itemTop < menu.scrollTop) {
    menu.scrollTop = itemTop
    return
  }
  if (itemBottom > menu.scrollTop + menu.clientHeight) {
    menu.scrollTop = itemBottom - menu.clientHeight
  }
})

function setItemRef(element: Element | ComponentPublicInstance | null, index: number) {
  if (element instanceof HTMLElement) itemRefs.value[index] = element
}

/** 文件来源：优先展示产出步骤，CLI 直接执行等无归属的文件退化为所在目录 */
function sourceLabel(file: DocumentMentionCandidate) {
  if (file.owner_step_name) return file.owner_step_name
  const directory = file.path
    .replace(/\\/g, '/')
    .split('/')
    .filter(Boolean)
    .slice(0, -1)
    .join('/')
  return directory || t('workflows.task.progress.taskRoot')
}

function selectFile(file: DocumentMentionCandidate) {
  emit('select', file)
}

function handleKeydown(event: KeyboardEvent) {
  if (!props.open) return false
  if (event.key === 'Escape') {
    event.preventDefault()
    emit('close')
    return true
  }
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    const count = props.items.length
    if (!count) return true
    const step = event.key === 'ArrowDown' ? 1 : -1
    activeIndex.value = (activeIndex.value + step + count) % count
    return true
  }
  if (event.key === 'Enter' || event.key === 'Tab') {
    const file = props.items[activeIndex.value]
    // 加载中或加载失败时没有候选，交回外层让 Enter 继续提交消息
    if (!file) return false
    event.preventDefault()
    selectFile(file)
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
  /* 贴住输入框上沿展开：用 100% 跟随容器高度，输入框随内容变高时不会被遮挡 */
  bottom: calc(100% + 8px);
  left: 24px;
  width: min(340px, calc(100% - 48px));
  max-height: 248px;
  overflow-y: auto;
  overscroll-behavior: contain;
  border: 0.5px solid #e5e5e5;
  border-radius: 16px;
  background: #fff;
  box-shadow:
    0 4px 16px 0 rgba(0, 0, 0, 0.08),
    0 1px 4px 0 rgba(0, 0, 0, 0.04);
  transform-origin: bottom center;
}
.mention-pop-enter-active {
  transition:
    opacity 180ms ease-out,
    transform 180ms ease-out;
}
.mention-pop-leave-active {
  transition:
    opacity 140ms ease-in,
    transform 140ms ease-in;
}
.mention-pop-enter-from,
.mention-pop-leave-to {
  opacity: 0;
  transform: translateY(6px) scale(0.98);
}
.mention-title,
.mention-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  color: #8c95a8;
  font-size: 11px;
}
.mention-title {
  position: sticky;
  z-index: 1;
  top: 0;
  border-bottom: 1px solid #f0f0f0;
  background: #fff;
}
.mention-title-label {
  color: #595959;
  font-weight: 600;
}
.mention-title-meta {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mention-state {
  display: flex;
  min-height: 54px;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 12px;
  color: #8c95a8;
  font-size: 12px;
}
.mention-error {
  color: #fb363f;
}
.mention-error button {
  border: 0;
  color: #3157e2;
  background: transparent;
  cursor: pointer;
  font-family: inherit;
  font-size: 12px;
}
.mention-item {
  display: flex;
  width: calc(100% - 16px);
  margin: 0 8px;
  align-items: center;
  gap: 10px;
  padding: 7px 8px;
  border: 0;
  border-radius: 8px;
  color: #344054;
  background: transparent;
  cursor: pointer;
  font-family: inherit;
  text-align: left;
  transition: background-color 140ms ease;
}
.mention-item:hover,
.mention-item.active {
  background: #f2f2f2;
}
.mention-item:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: -2px;
}
.agent-badge {
  display: inline-flex;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  color: #3157e2;
  background: #e5efff;
  font-size: 11px;
  font-weight: 600;
}
.mention-copy {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}
.mention-name,
.mention-owner {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mention-name {
  color: #262626;
  font-size: 13px;
  font-weight: 500;
}
.mention-hit {
  color: #3157e2;
  font-weight: 600;
}
.mention-owner {
  color: #8c95a8;
  font-size: 11px;
}
.mention-type {
  flex: 0 0 auto;
  padding: 2px 6px;
  border-radius: 4px;
  color: #595959;
  background: #edeff2;
  font-size: 10px;
}
.mention-footer {
  position: sticky;
  bottom: 0;
  justify-content: flex-end;
  border-top: 1px solid #f0f0f0;
  background: #fff;
}
@media (prefers-reduced-motion: reduce) {
  .mention-pop-enter-active,
  .mention-pop-leave-active,
  .mention-item {
    transition: none;
  }
}
</style>
