<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  DeleteOutlined,
  EditOutlined,
  FolderOpenOutlined,
  PlusOutlined,
} from '@ant-design/icons-vue'
import apiClient from '@/api/client'
import { isDesktopRuntime, selectDirectory } from '@/composables/useDesktop'
import { PROJECT_ICON_PRESETS, projectIconPreset } from '@/constants/project-icons'
import type { LocalProject, ProjectIconKind } from '@/types/project'

const loading = ref(false)
const saving = ref(false)
const projects = ref<LocalProject[]>([])
const modalOpen = ref(false)
const editing = ref<LocalProject>()
const form = reactive({
  name: '',
  icon_type: 'folder' as ProjectIconKind,
  icon_url: '',
  local_dir: '',
})

function formatTime(value?: number) {
  return value ? new Date(value < 1e12 ? value * 1000 : value).toLocaleDateString('zh-CN') : ''
}

async function load() {
  loading.value = true
  try {
    projects.value = (await apiClient.get<{ items: LocalProject[] }>('/projects')).items || []
  } catch (error) {
    message.error(error instanceof Error ? error.message : '项目加载失败')
  } finally {
    loading.value = false
  }
}

function openModal(project?: LocalProject) {
  editing.value = project
  form.name = project?.name || ''
  form.icon_type = project?.icon_type || 'folder'
  form.icon_url = project?.icon_url || ''
  form.local_dir = project?.local_dir || ''
  modalOpen.value = true
}

async function chooseDirectory() {
  try {
    const value = await selectDirectory(form.local_dir)
    if (value) form.local_dir = value
  } catch (error) {
    message.error(error instanceof Error ? error.message : '无法打开目录选择器')
  }
}

async function save() {
  if (!form.name.trim()) return message.warning('请输入项目名称')
  if (!form.local_dir.trim()) return message.warning('请选择项目本地目录')
  if (form.icon_type === 'custom' && !form.icon_url.trim())
    return message.warning('请输入自定义图标 URL')
  saving.value = true
  try {
    const payload = {
      name: form.name.trim(),
      icon_type: form.icon_type,
      icon_url: form.icon_type === 'custom' ? form.icon_url.trim() : '',
      local_dir: form.local_dir.trim(),
    }
    if (editing.value) await apiClient.put(`/projects/${editing.value.uuid}`, payload)
    else await apiClient.post('/projects', payload)
    modalOpen.value = false
    message.success(editing.value ? '项目已更新' : '项目已创建')
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '项目保存失败')
  } finally {
    saving.value = false
  }
}

function remove(project: LocalProject) {
  Modal.confirm({
    title: `删除项目“${project.name}”？`,
    content: '仅删除项目配置，不会删除本地目录和已有任务。',
    okType: 'danger',
    onOk: async () => {
      await apiClient.delete(`/projects/${project.uuid}`)
      await load()
    },
  })
}

onMounted(load)
</script>

<template>
  <div class="projects-page">
    <header class="page-bar">
      <div>
        <h1>项目</h1>
        <i /> <small>管理本地可用项目</small>
      </div>
    </header>
    <main class="projects-content">
      <div class="content-title">
        <h2>
          <FolderOpenOutlined />本地项目 <em>{{ projects.length }} 个</em>
        </h2>
        <a-button
          type="primary"
          size="small"
          @click="openModal()"
          ><template #icon><PlusOutlined /></template>新建项目</a-button
        >
      </div>
      <div class="project-guide">
        <span>i</span>
        <div>
          <strong>项目用于绑定本地目录作为项目目录</strong>
          <p>
            新建任务时选择“所属项目”，会自动将该项目目录作为主项目目录；选择多个子项目时，也会自动把对应目录加入任务，方便
            Agent 在正确的代码目录中工作。
          </p>
        </div>
      </div>
      <a-spin :spinning="loading">
        <div class="project-grid">
          <article
            v-for="project in projects"
            :key="project.uuid"
            class="project-card"
          >
            <div class="card-top">
              <img
                v-if="project.icon_type === 'custom' && project.icon_url"
                :src="project.icon_url"
                alt=""
                class="project-image"
              />
              <span
                v-else
                class="project-symbol"
                :style="{
                  color: projectIconPreset(project.icon_type).color,
                  background: projectIconPreset(project.icon_type).background,
                }"
                >{{ projectIconPreset(project.icon_type).symbol }}</span
              >
              <div>
                <strong>{{ project.name }}</strong
                ><small>绑定本地目录，任务关联时作为工作目录</small>
              </div>
              <a-dropdown
                ><a-button
                  type="text"
                  size="small"
                  >•••</a-button
                ><template #overlay
                  ><a-menu
                    ><a-menu-item @click="openModal(project)"><EditOutlined /> 编辑</a-menu-item
                    ><a-menu-item
                      danger
                      @click="remove(project)"
                      ><DeleteOutlined /> 删除</a-menu-item
                    ></a-menu
                  ></template
                ></a-dropdown
              >
            </div>
            <div class="directory">
              <FolderOpenOutlined /><span :title="project.local_dir">{{ project.local_dir }}</span
              ><em>项目目录</em>
            </div>
            <footer>
              <span>本地项目</span
              ><time v-if="project.created_at">创建于 {{ formatTime(project.created_at) }}</time>
            </footer>
          </article>
        </div>
        <a-empty
          v-if="!loading && !projects.length"
          description="暂无项目，点击右上角“新建项目”创建"
        />
      </a-spin>
    </main>

    <a-modal
      v-model:open="modalOpen"
      :title="editing ? '编辑项目' : '新建项目'"
      width="600px"
      :confirm-loading="saving"
      @ok="save"
    >
      <a-form layout="vertical"
        ><a-form-item
          label="项目名称（最多30个字）"
          required
          ><a-input
            v-model:value="form.name"
            :maxlength="30"
            show-count
            placeholder="请输入项目名称" /></a-form-item
        ><a-form-item
          label="项目图标"
          required
          ><div class="icon-options">
            <button
              v-for="icon in PROJECT_ICON_PRESETS"
              :key="icon.type"
              type="button"
              :class="{ active: form.icon_type === icon.type }"
              @click="form.icon_type = icon.type"
            >
              <span :style="{ color: icon.color, background: icon.background }">{{
                icon.symbol
              }}</span></button
            ><button
              type="button"
              :class="{ active: form.icon_type === 'custom' }"
              @click="form.icon_type = 'custom'"
            >
              <span class="custom">▧</span>
            </button>
          </div>
          <div
            v-if="form.icon_type === 'custom'"
            class="custom-icon-row"
          >
            <a-input
              v-model:value="form.icon_url"
              placeholder="粘贴图片 URL（支持 jpg / png / svg）"
            /><span
              ><img
                v-if="form.icon_url"
                :src="form.icon_url"
                alt=""
              /><template v-else>▧</template></span
            >
          </div></a-form-item
        ><a-form-item
          label="项目目录（绑定本地目录，选择文件夹）"
          required
          ><a-input-group compact
            ><a-input
              v-model:value="form.local_dir"
              :readonly="isDesktopRuntime()"
              style="width: calc(100% - 96px)"
              placeholder="请选择项目本地目录"
            /><a-button
              style="width: 96px"
              :disabled="!isDesktopRuntime()"
              @click="chooseDirectory"
              ><FolderOpenOutlined />选择目录</a-button
            ></a-input-group
          ></a-form-item
        ></a-form
      >
    </a-modal>
  </div>
</template>

<style scoped>
.projects-page {
  display: flex;
  height: calc(100% + 48px);
  margin: -24px;
  flex-direction: column;
  color: #1f2937;
  background: #f7f8fa;
}
.page-bar {
  display: flex;
  min-height: 44px;
  flex: 0 0 44px;
  align-items: center;
  padding: 0 24px;
  border-bottom: 1px solid #f0f0f0;
  background: #fff;
}
.page-bar > div {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
}
.page-icon {
  display: flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  color: #fff;
  background: #3157e2;
  font-size: 11px;
}
.page-bar i {
  width: 4px;
  height: 4px;
  margin-left: 4px;
  border-radius: 50%;
  background: #d1d5db;
}
.page-bar small {
  color: #9ca3af;
  font-size: 12px;
  font-weight: 400;
}
.projects-content {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  padding: 24px;
  background: #fff;
}
.content-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}
.content-title h2 {
  display: flex;
  align-items: center;
  gap: 7px;
  margin: 0;
  font-size: 14px;
}
.content-title h2 em {
  padding: 2px 7px;
  border-radius: 4px;
  color: #6b7280;
  background: #f0f2f5;
  font-size: 11px;
  font-style: normal;
}
.project-guide {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-bottom: 16px;
  padding: 12px 16px;
  border: 1px solid rgba(49, 87, 226, 0.15);
  border-radius: 8px;
  background: rgba(49, 87, 226, 0.04);
}
.project-guide > span {
  display: flex;
  width: 16px;
  height: 16px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  color: #fff;
  background: #3157e2;
  font-size: 10px;
  font-weight: 700;
}
.project-guide strong {
  font-size: 12px;
}
.project-guide p {
  margin: 3px 0 0;
  color: #6b7280;
  font-size: 12px;
  line-height: 18px;
}
.project-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}
.project-card {
  padding: 20px;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
  transition: 0.18s;
}
.project-card:hover {
  box-shadow: 0 4px 13px rgba(15, 23, 42, 0.08);
}
.card-top {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}
.project-symbol,
.project-image {
  display: flex;
  width: 40px;
  height: 40px;
  flex: 0 0 40px;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  object-fit: cover;
  font-size: 16px;
  font-weight: 700;
}
.card-top > div {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
}
.card-top strong {
  overflow: hidden;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.card-top small {
  margin-top: 3px;
  color: #9ca3af;
  font-size: 11px;
}
.directory {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-top: 14px;
  padding: 7px 9px;
  border-radius: 6px;
  color: #9ca3af;
  background: #f8fafc;
}
.directory span {
  overflow: hidden;
  min-width: 0;
  flex: 1;
  color: #4b5563;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.directory em {
  flex: 0 0 auto;
  padding: 1px 6px;
  border-radius: 4px;
  color: #3157e2;
  background: #eef2ff;
  font-size: 10px;
  font-style: normal;
}
.project-card footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 13px;
  padding-top: 12px;
  border-top: 1px solid #f0f2f5;
  color: #c0c5ce;
  font-size: 11px;
}
.icon-options {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.icon-options button {
  width: 46px;
  height: 46px;
  padding: 2px;
  border: 2px solid transparent;
  border-radius: 10px;
  background: #fff;
  cursor: pointer;
}
.icon-options button.active {
  border-color: #3157e2;
}
.icon-options span {
  display: flex;
  width: 38px;
  height: 38px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  font-weight: 700;
}
.icon-options .custom {
  color: #9ca3af;
  background: #f8fafc;
}
.custom-icon-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}
.custom-icon-row > span {
  display: flex;
  width: 40px;
  height: 40px;
  flex: 0 0 40px;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border: 1px dashed #d1d5db;
  border-radius: 8px;
  color: #cbd5e1;
  background: #f8fafc;
}
.custom-icon-row img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
@media (max-width: 1100px) {
  .project-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 700px) {
  .project-grid {
    grid-template-columns: 1fr;
  }
  .page-bar small,
  .page-bar i {
    display: none;
  }
}
.page-bar {
  border-bottom-color: #f0f0f0;
}
.page-bar h1 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  line-height: 28px;
}
</style>
