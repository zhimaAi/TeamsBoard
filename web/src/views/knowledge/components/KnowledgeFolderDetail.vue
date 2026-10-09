<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  DeleteOutlined,
  FileTextOutlined,
  FolderAddOutlined,
  FolderOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  PlusOutlined,
  SearchOutlined,
  SortAscendingOutlined,
  SortDescendingOutlined,
} from '@ant-design/icons-vue'
import type { KnowledgeDocument, KnowledgeFolderNode } from '@/api/knowledge'
import { useAppI18n } from '@/i18n'

// S-UI-14: 文件夹详情列表页（与「回收站」「编辑器」并列的独立主区工作区）
// S-DA-11: 搜索与排序为纯前端行为，不写 localStorage、不调接口
const { t, locale } = useAppI18n()

type BreadcrumbItem = { id: number; name: string }
type SortKey = 'name' | 'updated_at'

/**
 * 名称列的「本地」占位由词条固定渲染；类型上预留远程登录人的名称与 id（S-UI-15 / S-DA-10，
 * 本次不新增数据库列，远程字段仅在此预留结构）。
 */
type DetailOwner = { id: string; name: string }

const props = defineProps<{
  folderId: number
  folderName: string
  breadcrumbs: BreadcrumbItem[]
  childFolders: KnowledgeFolderNode[]
  childDocuments: KnowledgeDocument[]
  loading?: boolean
  /** 父级 sidebar 折叠状态，用于切换按钮图标 */
  sidebarCollapsed?: boolean
}>()

const emit = defineEmits<{
  'open-folder': [folderId: number]
  'open-document': [uuid: string]
  'create-document': [folderId: number]
  'create-txt-document': [folderId: number]
  'create-folder': [folderId: number]
  'delete-folder': [folderId: number]
  'open-create-modal': [type: 'document' | 'folder', folderId: number]
  'toggle-inspector': []
  /** 触发父级折叠 / 展开左侧目录栏 */
  'toggle-sidebar': []
  navigate: [folderId: number]
}>()

const keyword = ref('')
const sortKey = ref<SortKey>('updated_at')
const sortAsc = ref(false)

// 模板中的裸标点会被 @intlify/vue-i18n/no-raw-text 判为未国际化文本，故用常量插值渲染分隔符
const crumbSeparator = '/'

type DetailRow =
  | { key: string; kind: 'folder'; folderId: number; name: string; updatedAt: number }
  | { key: string; kind: 'document'; uuid: string; name: string; updatedAt: number }

const rows = computed<DetailRow[]>(() => {
  const folderRows = props.childFolders.map<DetailRow>((folder) => ({
    key: `folder:${folder.id}`,
    kind: 'folder',
    folderId: folder.id,
    name: folder.name,
    updatedAt: folder.updated_at ?? 0,
  }))
  const documentRows = props.childDocuments.map<DetailRow>((document) => ({
    key: `doc:${document.uuid}`,
    kind: 'document',
    uuid: document.uuid,
    name: document.ext ? `${document.title}.${document.ext}` : document.title,
    updatedAt: document.updated_at ?? 0,
  }))
  const text = keyword.value.trim().toLowerCase()
  const matched = text
    ? [...folderRows, ...documentRows].filter((row) => row.name.toLowerCase().includes(text))
    : [...folderRows, ...documentRows]
  const direction = sortAsc.value ? 1 : -1
  return matched.sort((left, right) => {
    if (sortKey.value === 'name') {
      return direction * left.name.localeCompare(right.name, locale.value)
    }
    if (left.updatedAt === right.updatedAt) {
      // Minor-6：主排序平级时才补「文件夹优先」次级键，且不参与升降序方向，避免同组内顺序被反向
      if (left.kind !== right.kind) return left.kind === 'folder' ? -1 : 1
      return left.name.localeCompare(right.name, locale.value)
    }
    return direction * (left.updatedAt - right.updatedAt)
  })
})

const sortDirectionLabel = computed(() =>
  sortAsc.value ? t('knowledge.detailSortDesc') : t('knowledge.detailSortAsc'),
)

// CF-4: 文档表无「最近访问」字段，按既有约定以 updated_at 承载展示
function formatTime(value: number) {
  if (!value) return '—'
  return new Date(value).toLocaleString(locale.value, { hour12: false })
}

function ownerLabel(owner?: DetailOwner | null) {
  return owner?.name || t('knowledge.detailOwnerLocal')
}

function openRow(row: DetailRow) {
  if (row.kind === 'folder') {
    emit('open-folder', row.folderId)
    return
  }
  emit('open-document', row.uuid)
}

function toggleSortDirection() {
  sortAsc.value = !sortAsc.value
}
</script>

<template>
  <main class="folder-detail">
    <!-- S-UI-14: 面包屑「上级文件夹 / 当前文件夹」，点击根节点回到空态；右侧抽屉按钮（默认收起） -->
    <nav class="detail-breadcrumb">
      <div class="detail-breadcrumb-leading">
        <!-- 左侧目录栏折叠 / 展开切换（固定在面包屑最左侧） -->
        <a-tooltip
          :title="(props.sidebarCollapsed ?? false) ? t('knowledge.expandSidebar') : t('knowledge.collapseSidebar')"
        >
          <button
            class="icon-button sidebar-nav-toggle"
            type="button"
            :aria-label="(props.sidebarCollapsed ?? false) ? t('knowledge.expandSidebar') : t('knowledge.collapseSidebar')"
            @click="emit('toggle-sidebar')"
          >
            <component :is="(props.sidebarCollapsed ?? false) ? MenuUnfoldOutlined : MenuFoldOutlined" />
          </button>
        </a-tooltip>
        <div class="detail-breadcrumb-path">
          <template v-for="(item, index) in breadcrumbs" :key="item.id">
            <button
              v-if="index < breadcrumbs.length - 1"
              type="button"
              class="crumb crumb-link"
              @click="emit('navigate', item.id)"
            >
              {{ item.name }}
            </button>
            <span v-else class="crumb crumb-current">{{ item.name }}</span>
            <span
              v-if="index < breadcrumbs.length - 1"
              class="crumb-separator"
            >{{ crumbSeparator }}</span>
          </template>
        </div>
      </div>
      <div class="detail-breadcrumb-actions">
        <!-- 抽屉展开 / 折叠切换（固定在面包屑右侧，默认收起） -->
        <a-tooltip :title="t('knowledge.inspectorExpand')">
          <button
            class="icon-button"
            type="button"
            :aria-label="t('knowledge.inspectorExpand')"
            @click="emit('toggle-inspector')"
          >
            <MenuUnfoldOutlined />
          </button>
        </a-tooltip>
      </div>
    </nav>

    <header class="detail-header">
      <div class="detail-title-section">
        <span class="detail-folder-icon"><FolderOutlined /></span>
        <h2 class="detail-title">{{ folderName }}</h2>
      </div>
      <div class="detail-actions">
        <a-tooltip :title="t('knowledge.deleteFolder')">
          <button
            class="icon-button danger-button"
            type="button"
            :aria-label="t('knowledge.deleteFolder')"
            @click="emit('delete-folder', folderId)"
          >
            <DeleteOutlined />
          </button>
        </a-tooltip>
        <!-- S-UI-14: 右上「快速添加」；新建 md/文件夹走弹窗输入名称，txt 走直接创建 -->
        <a-dropdown :trigger="['click']">
          <button class="quick-add-button" type="button">
            <PlusOutlined />
            <span>{{ t('knowledge.quickAdd') }}</span>
          </button>
          <template #overlay>
            <a-menu>
              <a-menu-item key="md-document" @click="emit('open-create-modal', 'document', folderId)">
                <FileTextOutlined /> {{ t('knowledge.newMdDocument') }}
              </a-menu-item>
              <a-menu-item key="folder" @click="emit('open-create-modal', 'folder', folderId)">
                <FolderAddOutlined /> {{ t('knowledge.newFolder') }}
              </a-menu-item>
              <a-sub-menu key="more">
                <template #title>{{ t('knowledge.more') }}</template>
                <a-menu-item key="txt-document" @click="emit('create-txt-document', folderId)">
                  <FileTextOutlined /> {{ t('knowledge.newTxtDocument') }}
                </a-menu-item>
              </a-sub-menu>
            </a-menu>
          </template>
        </a-dropdown>
      </div>
    </header>

    <div class="detail-toolbar">
      <a-input
        v-model:value="keyword"
        class="detail-search"
        allow-clear
        :placeholder="t('knowledge.detailSearchPlaceholder')"
      >
        <template #suffix><SearchOutlined /></template>
      </a-input>
      <!-- S-UI-16: 列表内搜索（按名称过滤）与排序（按名称 / 按更新时间），均为前端本地行为 -->
      <a-select v-model:value="sortKey" class="detail-sort-select">
        <a-select-option value="updated_at">{{ t('knowledge.sortByUpdatedAt') }}</a-select-option>
        <a-select-option value="name">{{ t('knowledge.sortByName') }}</a-select-option>
      </a-select>
      <a-tooltip :title="sortDirectionLabel">
        <button
          class="icon-button"
          type="button"
          :aria-label="sortDirectionLabel"
          @click="toggleSortDirection"
        >
          <SortAscendingOutlined v-if="sortAsc" />
          <SortDescendingOutlined v-else />
        </button>
      </a-tooltip>
    </div>

    <div class="detail-content">
      <a-spin :spinning="Boolean(loading)">
        <table class="detail-table">
          <thead>
            <tr>
              <th scope="col" class="col-name">
                {{ t('knowledge.detailName') }}
                <span class="detail-count">{{ rows.length }}</span>
              </th>
              <th scope="col" class="col-owner">{{ t('knowledge.detailOwnerLocal') }}</th>
              <th scope="col" class="col-time">{{ t('knowledge.detailLastVisited') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in rows" :key="row.key" class="detail-row">
              <td class="col-name">
                <button type="button" class="detail-row-name" @click="openRow(row)">
                  <FolderOutlined v-if="row.kind === 'folder'" class="row-folder-icon" />
                  <FileTextOutlined v-else class="row-document-icon" />
                  <span class="row-title">{{ row.name }}</span>
                </button>
              </td>
              <td class="col-owner">{{ ownerLabel() }}</td>
              <td class="col-time">{{ formatTime(row.updatedAt) }}</td>
            </tr>
          </tbody>
        </table>
        <!-- 边界 E15：空文件夹显示空态，不报错 -->
        <div v-if="!rows.length" class="detail-empty">{{ t('knowledge.detailEmpty') }}</div>
      </a-spin>
    </div>
  </main>
</template>

<style scoped>
.folder-detail {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  background: #fff;
}

.detail-breadcrumb {
  display: flex;
  height: 48px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 24px;
  border-bottom: 1px solid #f0f0f0;
  font-size: 14px;
}

.detail-breadcrumb-path {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  overflow: hidden;
}

.detail-breadcrumb-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
}

/* 面包屑行左侧容器：包含 sidebar 折叠按钮 + 路径导航 */
.detail-breadcrumb-leading {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  overflow: hidden;
}

.sidebar-nav-toggle {
  flex: 0 0 auto;
}

.crumb {
  padding: 0;
  border: 0;
  color: #8c8c8c;
  background: transparent;
  font-size: 14px;
}

.crumb-link {
  cursor: pointer;
}

.crumb-link:hover {
  color: #3157e2;
}

.crumb-link:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.crumb-current {
  color: #262626;
  font-weight: 600;
}

.crumb-separator {
  color: #bfbfbf;
}

.detail-header {
  display: flex;
  min-width: 0;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 24px;
  border-bottom: 1px solid #f0f0f0;
}

.detail-title-section {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
}

.detail-folder-icon {
  display: flex;
  width: 32px;
  height: 32px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: #3157e2;
  background: #e5efff;
  font-size: 16px;
}

.detail-title {
  overflow: hidden;
  margin: 0;
  color: #262626;
  font-size: 18px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
}

.quick-add-button {
  display: inline-flex;
  height: 32px;
  align-items: center;
  gap: 6px;
  padding: 0 14px;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  color: #595959;
  background: #fff;
  cursor: pointer;
  font-size: 14px;
  transition: color 0.18s ease, border-color 0.18s ease, background 0.18s ease;
}

.quick-add-button:hover {
  border-color: #3157e2;
  color: #3157e2;
  background: #f9fafb;
}

.quick-add-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.icon-button {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 8px;
  color: #595959;
  background: transparent;
  cursor: pointer;
  font-size: 16px;
  transition: color 0.18s ease, background 0.18s ease;
}

.icon-button:hover {
  color: #3157e2;
  background: #f0f1f3;
}

.icon-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.danger-button:hover {
  color: #fb363f;
  background: #fef2f2;
}

.detail-toolbar {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  padding: 10px 24px 0;
}

.detail-search {
  max-width: 280px;
}

.detail-sort-select {
  width: 168px;
}

.detail-content {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  padding: 0 24px 24px;
}

.detail-content :deep(.ant-spin-nested-loading),
.detail-content :deep(.ant-spin-container) {
  display: block;
  width: 100%;
}

.detail-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}

.detail-table th {
  height: 48px;
  border-bottom: 1px solid #f0f0f0;
  color: #8c8c8c;
  font-size: 14px;
  font-weight: 400;
  text-align: left;
}

.detail-table th.col-name {
  padding-left: 8px;
}

.detail-table .col-owner,
.detail-table .col-time {
  width: 180px;
}

.detail-count {
  margin-left: 4px;
  color: #bfbfbf;
}

.detail-row {
  height: 56px;
  border-bottom: 1px solid #f0f0f0;
  color: #595959;
  font-size: 14px;
}

.detail-row:hover {
  background: #f9fafb;
}

.detail-row td {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-row td.col-name {
  padding-left: 8px;
}

.detail-row-name {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: center;
  gap: 10px;
  padding: 0;
  border: 0;
  color: #262626;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  text-align: left;
}

.detail-row-name:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.row-folder-icon {
  flex: 0 0 auto;
  color: #3157e2;
  font-size: 16px;
}

.row-document-icon {
  flex: 0 0 auto;
  color: #8c8c8c;
  font-size: 16px;
}

.row-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-row .col-owner {
  color: #8c8c8c;
}

.detail-row .col-time {
  color: #8c8c8c;
  font-variant-numeric: tabular-nums;
}

.detail-empty {
  padding: 64px 12px;
  color: #8c8c8c;
  font-size: 14px;
  text-align: center;
}
</style>
