<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { SearchOutlined } from '@ant-design/icons-vue'
import {
  listKnowledgeDocuments,
  listKnowledgeFolders,
  type KnowledgeDocument,
} from '@/api/knowledge'
import { buildKnowledgeFolderPaths } from '@/composables/useKnowledgeReferences'
import { useAppI18n } from '@/i18n'

/**
 * S-IN-11 / S-UI-20：「从资料库中选择」选择器。
 *
 * 数据来自既有的 `/knowledge/documents` + `/knowledge/folders`，不新增列表接口；
 * 搜索为前端本地名称过滤（与目录树搜索放行口径一致，避免每次输入都打接口）。
 */
const { t, locale } = useAppI18n()

const props = defineProps<{
  open: boolean
  /** 已在输入框里引用过的知识库文档（去重标记，行内禁用并标注「已引用」） */
  referencedUuids?: string[]
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  confirm: [documents: KnowledgeDocument[]]
}>()

const loading = ref(false)
const loadError = ref('')
const documents = ref<KnowledgeDocument[]>([])
const folderPaths = ref<Map<number, string>>(new Map())
const keyword = ref('')
const selectedUuids = ref<Set<string>>(new Set())

const referenced = computed(() => new Set(props.referencedUuids ?? []))

const filteredDocuments = computed(() => {
  const text = keyword.value.trim().toLowerCase()
  if (!text) return documents.value
  return documents.value.filter((document) => document.title.toLowerCase().includes(text))
})

const selectableDocuments = computed(() =>
  filteredDocuments.value.filter((document) => !referenced.value.has(document.uuid)),
)

const selectedCount = computed(() => selectedUuids.value.size)
const allSelected = computed(
  () =>
    selectableDocuments.value.length > 0 &&
    selectableDocuments.value.every((document) => selectedUuids.value.has(document.uuid)),
)

/** 所属文件夹路径：根（folder_id = 0）显示为「默认文件夹」 */
function locationOf(document: KnowledgeDocument) {
  if (!document.folder_id) return ''
  return folderPaths.value.get(document.folder_id) || ''
}

function formatTime(value: number | undefined) {
  if (!value) return '—'
  return new Date(value).toLocaleString(locale.value, { hour12: false })
}

function isSelected(uuid: string) {
  return selectedUuids.value.has(uuid)
}

function isReferenced(uuid: string) {
  return referenced.value.has(uuid)
}

function toggleSelection(uuid: string) {
  const next = new Set(selectedUuids.value)
  if (next.has(uuid)) next.delete(uuid)
  else next.add(uuid)
  selectedUuids.value = next
}

function toggleAll() {
  if (allSelected.value) {
    selectedUuids.value = new Set()
    return
  }
  selectedUuids.value = new Set(selectableDocuments.value.map((document) => document.uuid))
}

function close() {
  emit('update:open', false)
}

function confirmSelection() {
  if (!selectedCount.value) return
  const picked = documents.value.filter((document) => selectedUuids.value.has(document.uuid))
  emit('confirm', picked)
  close()
}

async function loadLibrary() {
  loading.value = true
  loadError.value = ''
  try {
    // 两个列表互不依赖：任一失败都要保留可用的另一份数据
    const [documentsResult, foldersResult] = await Promise.allSettled([
      listKnowledgeDocuments(),
      listKnowledgeFolders(),
    ])
    if (documentsResult.status === 'fulfilled') {
      documents.value = documentsResult.value.data || []
    } else {
      documents.value = []
      loadError.value =
        documentsResult.reason instanceof Error ? documentsResult.reason.message : ''
    }
    if (foldersResult.status === 'fulfilled') {
      folderPaths.value = buildKnowledgeFolderPaths(foldersResult.value.data || [])
    } else {
      folderPaths.value = new Map()
    }
  } finally {
    loading.value = false
  }
}

watch(
  () => props.open,
  (open) => {
    if (!open) return
    keyword.value = ''
    selectedUuids.value = new Set()
    void loadLibrary()
  },
)
</script>

<template>
  <a-modal
    :open="open"
    :title="t('workflows.task.progress.chooseKnowledgeDocuments')"
    :width="720"
    :footer="null"
    @cancel="close"
  >
    <div class="knowledge-picker">
      <a-input
        v-model:value="keyword"
        class="knowledge-picker-search"
        allow-clear
        :placeholder="t('workflows.task.progress.knowledgeSearchPlaceholder')"
      >
        <template #suffix><SearchOutlined /></template>
      </a-input>

      <a-spin :spinning="loading">
        <div v-if="loadError" class="knowledge-picker-state">
          <span>{{ loadError || t('workflows.task.progress.knowledgeLoadFailed') }}</span>
          <a-button type="link" size="small" @click="loadLibrary">
            {{ t('common.actions.retry') }}
          </a-button>
        </div>
        <table v-else class="knowledge-picker-table">
          <thead>
            <tr>
              <th scope="col" class="col-check">
                <a-checkbox
                  :checked="allSelected"
                  :disabled="!selectableDocuments.length"
                  :aria-label="t('workflows.task.progress.knowledgeSelectAll')"
                  @change="toggleAll"
                />
              </th>
              <th scope="col" class="col-name">
                {{ t('knowledge.detailName') }}
                <span class="knowledge-picker-count">{{ filteredDocuments.length }}</span>
              </th>
              <th scope="col" class="col-owner">
                {{ t('workflows.task.progress.knowledgeOwnerLocal') }}
              </th>
              <th scope="col" class="col-location">
                {{ t('workflows.task.progress.knowledgeLocation') }}
              </th>
              <th scope="col" class="col-time">
                {{ t('workflows.task.progress.knowledgeLastVisited') }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="document in filteredDocuments"
              :key="document.uuid"
              class="knowledge-picker-row"
            >
              <td class="col-check">
                <a-checkbox
                  :checked="isSelected(document.uuid)"
                  :disabled="isReferenced(document.uuid)"
                  :aria-label="document.title"
                  @change="toggleSelection(document.uuid)"
                />
              </td>
              <td class="col-name">
                <span class="knowledge-picker-name">{{ document.title }}</span>
                <span v-if="isReferenced(document.uuid)" class="knowledge-picker-tag">
                  {{ t('workflows.task.progress.knowledgeReferenced') }}
                </span>
              </td>
              <!-- S-UI-15/S-UI-20：本地阶段所有者列固定显示「本地」，不渲染真实人名 -->
              <td class="col-owner">{{ t('workflows.task.progress.knowledgeOwnerLocal') }}</td>
              <td class="col-location">{{ locationOf(document) }}</td>
              <!-- CF-4：文档表无「最近访问」字段，按既有约定以 updated_at 承载展示 -->
              <td class="col-time">{{ formatTime(document.updated_at) }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="!loading && !loadError && !filteredDocuments.length" class="knowledge-picker-state">
          {{ t('workflows.task.progress.knowledgeEmpty') }}
        </div>
      </a-spin>

      <div class="knowledge-picker-footer">
        <span class="knowledge-picker-selected">
          {{ t('workflows.task.progress.knowledgeSelectedCount', { count: selectedCount }) }}
        </span>
        <div class="knowledge-picker-actions">
          <a-button @click="close">{{ t('common.actions.cancel') }}</a-button>
          <a-button type="primary" :disabled="!selectedCount" @click="confirmSelection">
            {{ t('common.actions.confirm') }}
          </a-button>
        </div>
      </div>
    </div>
  </a-modal>
</template>

<style scoped>
.knowledge-picker {
  display: flex;
  flex-direction: column;
}

.knowledge-picker-search {
  max-width: 260px;
  margin-bottom: 12px;
}

.knowledge-picker-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}

.knowledge-picker-table th {
  height: 44px;
  border-bottom: 1px solid #f0f0f0;
  color: #8c8c8c;
  font-size: 13px;
  font-weight: 400;
  text-align: left;
}

.knowledge-picker-table .col-check {
  width: 44px;
  padding-left: 4px;
}

.knowledge-picker-table .col-owner,
.knowledge-picker-table .col-time {
  width: 150px;
}

.knowledge-picker-table .col-location {
  width: 170px;
}

.knowledge-picker-count {
  margin-left: 4px;
  color: #bfbfbf;
}

.knowledge-picker-row {
  height: 52px;
  border-bottom: 1px solid #f0f0f0;
  color: #595959;
  font-size: 13px;
}

.knowledge-picker-row:hover {
  background: #f9fafb;
}

.knowledge-picker-row td {
  overflow: hidden;
  padding-right: 8px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.knowledge-picker-name {
  color: #262626;
}

.knowledge-picker-tag {
  margin-left: 8px;
  padding: 1px 6px;
  border-radius: 999px;
  color: #8c95a8;
  background: #edeff2;
  font-size: 11px;
}

.knowledge-picker-row .col-time {
  color: #8c8c8c;
  font-variant-numeric: tabular-nums;
}

.knowledge-picker-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 48px 12px;
  color: #8c8c8c;
  font-size: 13px;
}

.knowledge-picker-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 16px;
}

.knowledge-picker-selected {
  color: #8c8c8c;
  font-size: 13px;
}

.knowledge-picker-actions {
  display: flex;
  gap: 8px;
}
</style>
