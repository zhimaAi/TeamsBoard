<template>
  <a-modal
    :open="open"
    class="project-modal"
    :title="project ? t('projects.editProject') : t('projects.newProject')"
    width="730px"
    @update:open="handleOpenChange"
  >

    <button
      class="project-icon-preview"
      type="button"
      :aria-label="t('projects.selectCustomIcon')"
      @click="openFilePicker"
    >
      <span
        class="project-icon-preview-image"
        :class="{ uploaded: hasUploadedImage }"
        :style="{ background: previewBackground }"
      >
        <img
          :src="previewIcon"
          alt=""
        />
      </span>
      <span class="project-icon-preview-camera">
        <img
          :src="projectModalCameraIcon"
          alt=""
          aria-hidden="true"
        />
      </span>
    </button>
    <p class="project-icon-preview-hint">{{ t('projects.iconHint') }}</p>

    <input
      ref="fileInput"
      class="project-icon-file-input"
      type="file"
      :accept="ICON_FILE_ACCEPT"
      @change="handleFileChange"
    />

    <a-form
      class="project-modal-form"
      layout="vertical"
    >
      <a-form-item
        class="project-icon-form-item"
        :label="t('projects.icon')"
      >
        <div class="project-icon-options">
          <button
            v-for="iconType in PROJECT_MODAL_ICON_ORDER"
            :key="iconType"
            type="button"
            :class="{ active: form.icon_type === iconType }"
            :aria-label="t('projects.selectPresetIcon', { type: iconType })"
            :aria-pressed="form.icon_type === iconType"
            @click="selectIcon(iconType)"
          >
            <span
              class="project-icon-option-image"
              :style="{ background: projectModalIconBackground(iconType) }"
            >
              <img
                :src="projectModalIcon(iconType)"
                alt=""
                aria-hidden="true"
              />
            </span>
          </button>
        </div>
      </a-form-item>

      <a-form-item
        class="project-modal-form-item"
        required
      >
        <template #label>
          <span class="project-modal-label">{{ t('projects.name') }}</span>
          <span class="project-modal-label-hint">{{ t('projects.nameLimit') }}</span>
        </template>
        <a-input
          v-model:value="form.name"
          :maxlength="30"
          :show-count="false"
          :placeholder="t('projects.namePlaceholder')"
        />
      </a-form-item>

      <a-form-item
        class="project-modal-form-item"
        required
      >
        <template #label>
          <span class="project-modal-label">{{ t('projects.directory') }}</span>
          <span class="project-modal-label-hint">{{ t('projects.directoryHint') }}</span>
        </template>
        <a-input
          v-model:value="form.local_dir"
          :readonly="isDesktopRuntime()"
          :placeholder="t('projects.directoryPlaceholder')"
        >
          <template #suffix>
            <button
              class="project-directory-select"
              type="button"
              :disabled="!isDesktopRuntime()"
              :aria-label="t('projects.selectDirectory')"
              @click.stop="chooseDirectory"
            >
              <img
                :src="projectModalDirectoryChevronIcon"
                alt=""
                aria-hidden="true"
              />
            </button>
          </template>
        </a-input>
      </a-form-item>
    </a-form>

    <template #footer>
      <div class="project-modal-footer-actions">
        <a-button @click="handleOpenChange(false)">{{ t('projects.cancel') }}</a-button>
        <a-button
          type="primary"
          :loading="saving"
          @click="save"
        >
          {{ t('projects.confirm') }}
        </a-button>
      </div>
    </template>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import apiClient from '@/api/client'
import projectModalCameraIcon from '@/assets/icons/project-modal-camera.svg'
import projectModalDirectoryChevronIcon from '@/assets/icons/project-modal-directory-chevron.svg'
import projectModalFolderIcon from '@/assets/icons/project-modal-folder.svg'
import projectModalCodeIcon from '@/assets/icons/project-modal-icon-code.svg'
import projectModalDatabaseIcon from '@/assets/icons/project-modal-icon-database.svg'
import projectModalFolderPresetIcon from '@/assets/icons/project-modal-icon-folder.svg'
import projectModalGlobeIcon from '@/assets/icons/project-modal-icon-globe.svg'
import projectModalImageIcon from '@/assets/icons/project-modal-icon-image.svg'
import projectModalMobileIcon from '@/assets/icons/project-modal-icon-mobile.svg'
import projectModalRocketIcon from '@/assets/icons/project-modal-icon-rocket.svg'
import { ICON_FILE_ACCEPT, useIconFile } from '@/composables/useIconFile'
import { isDesktopRuntime, selectDirectory } from '@/composables/useDesktop'
import type { LocalProject, ProjectIconKind } from '@/types/project'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

const props = defineProps<{
  open: boolean
  project?: LocalProject
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const PROJECT_MODAL_ICON_ASSETS: Record<ProjectIconKind, string> = {
  folder: projectModalFolderPresetIcon,
  rocket: projectModalRocketIcon,
  code: projectModalCodeIcon,
  database: projectModalDatabaseIcon,
  globe: projectModalGlobeIcon,
  mobile: projectModalMobileIcon,
  image: projectModalImageIcon,
  custom: projectModalImageIcon,
}

const PROJECT_MODAL_ICON_BACKGROUNDS: Record<ProjectIconKind, string> = {
  folder: '#e5efff',
  rocket: '#f8fbd9',
  code: '#e5ebff',
  database: '#f6efff',
  globe: '#dcf2f5',
  mobile: '#fff4e5',
  image: '#ffe6f6',
  custom: '#ffe6f6',
}

const PROJECT_MODAL_ICON_ORDER: Exclude<ProjectIconKind, 'custom'>[] = [
  'folder',
  'rocket',
  'database',
  'code',
  'globe',
  'mobile',
  'image',
]

const saving = ref(false)
const fileInput = ref<HTMLInputElement>()
const form = reactive({
  name: '',
  icon_type: 'folder' as ProjectIconKind,
  icon_url: '',
  local_dir: '',
})
const { file: iconFile, previewUrl, selectFile, reset: resetIconFile } = useIconFile()

const hasUploadedImage = computed(() => Boolean(previewUrl.value || form.icon_url))
const uploadedIcon = computed(
  () => previewUrl.value || form.icon_url || projectModalImageIcon,
)
const previewIcon = computed(() => {
  if (form.icon_type === 'custom') return uploadedIcon.value
  if (form.icon_type === 'folder') return projectModalFolderIcon
  return PROJECT_MODAL_ICON_ASSETS[form.icon_type]
})
const previewBackground = computed(() =>
  form.icon_type === 'custom' && hasUploadedImage.value
    ? '#f2f3f5'
    : projectModalIconBackground(form.icon_type),
)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    resetIconFile()
    form.name = props.project?.name || ''
    form.icon_type = props.project?.icon_type || 'folder'
    form.icon_url = props.project?.icon_url || ''
    form.local_dir = props.project?.local_dir || ''
  },
  { immediate: true },
)

function projectModalIcon(type: ProjectIconKind) {
  return PROJECT_MODAL_ICON_ASSETS[type]
}

function projectModalIconBackground(type: ProjectIconKind) {
  return PROJECT_MODAL_ICON_BACKGROUNDS[type]
}

function selectIcon(type: ProjectIconKind) {
  form.icon_type = type
}

function openFilePicker() {
  fileInput.value?.click()
}

function handleFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const nextFile = input.files?.[0]
  input.value = ''
  if (!nextFile) return

  try {
    selectFile(nextFile)
    form.icon_type = 'custom'
  } catch (error) {
    message.warning(error instanceof Error ? error.message : t('projects.selectImageFailed'))
  }
}

async function chooseDirectory() {
  try {
    const value = await selectDirectory(form.local_dir)
    if (value) form.local_dir = value
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('projects.openDirectoryFailed'))
  }
}

async function save() {
  if (!form.name.trim()) return message.warning(t('projects.namePlaceholder'))
  if (!form.local_dir.trim()) return message.warning(t('projects.directoryPlaceholder'))
  if (form.icon_type === 'custom' && !iconFile.value && !form.icon_url) {
    return message.warning(t('projects.selectImage'))
  }

  saving.value = true
  try {
    const path = props.project ? `/projects/${props.project.uuid}` : '/projects'
    if (form.icon_type === 'custom' && iconFile.value) {
      const payload = new FormData()
      payload.append('name', form.name.trim())
      payload.append('icon_type', form.icon_type)
      payload.append('local_dir', form.local_dir.trim())
      payload.append('icon_file', iconFile.value)
      if (props.project) await apiClient.put(path, payload)
      else await apiClient.post(path, payload)
    } else {
      const payload = {
        name: form.name.trim(),
        icon_type: form.icon_type,
        icon_url: form.icon_type === 'custom' ? form.icon_url : '',
        local_dir: form.local_dir.trim(),
      }
      if (props.project) await apiClient.put(path, payload)
      else await apiClient.post(path, payload)
    }
    message.success(props.project ? t('projects.updated') : t('projects.created'))
    emit('saved')
    handleOpenChange(false)
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('projects.saveFailed'))
  } finally {
    saving.value = false
  }
}

function handleOpenChange(open: boolean) {
  if (!open) resetIconFile()
  emit('update:open', open)
}
</script>

<style scoped>
.project-icon-preview {
  position: relative;
  display: flex;
  width: 100%;
  height: 88px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: pointer;
}

.project-icon-preview-image {
  display: flex;
  width: 60px;
  height: 60px;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: 12px;
  background: #e5efff;
}

.project-icon-preview-image img {
  display: block;
  width: 28px;
  height: 28px;
  object-fit: cover;
}

.project-icon-preview-image.uploaded img {
  width: 100%;
  height: 100%;
}

.project-icon-preview-camera {
  position: absolute;
  bottom: 4px;
  left: calc(50% + 8px);
  display: flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border: 0.75px solid #fff;
  border-radius: 12px;
  background: #262626;
  box-shadow: 0 2px 2px rgba(221, 221, 221, 0.12);
}

.project-icon-preview-camera img {
  display: block;
  width: 12px;
  height: 12px;
}

.project-icon-preview:focus-visible,
.project-icon-options button:focus-visible,
.project-directory-select:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.project-icon-preview-hint {
  margin: 0;
  text-align: center;
  color: rgba(0, 0, 0, 0.45);
  font-size: 14px;
  line-height: 22px;
}

.project-icon-file-input {
  position: fixed;
  width: 1px;
  height: 1px;
  overflow: hidden;
  opacity: 0;
  pointer-events: none;
}

.project-modal-form {
  margin-top: 24px;
}

.project-icon-form-item,
.project-modal-form-item {
  margin-bottom: 20px;
}

.project-modal-form-item:last-child {
  margin-bottom: 0;
}

.project-icon-options {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.project-icon-options button {
  display: flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  padding: 1px;
  border: 2px solid transparent;
  border-radius: 10px;
  background: #fff;
  cursor: pointer;
}

.project-icon-options button.active {
  border-color: #3157e2;
}

.project-icon-option-image {
  display: block;
  width: 28px;
  height: 28px;
  overflow: hidden;
  border-radius: 8px;
}

.project-icon-option-image img {
  display: block;
  width: 14px;
  height: 14px;
  margin: 7px;
}

.project-directory-select {
  display: flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: pointer;
}

.project-directory-select:disabled {
  cursor: not-allowed;
}

.project-directory-select img {
  display: block;
  width: 10.178px;
  height: 6.857px;
}

.project-modal-footer-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

:deep(.project-modal) {
  max-width: calc(100vw - 32px);
  padding-bottom: 0;
}

:deep(.project-modal .ant-modal-content) {
  overflow: hidden;
  padding: 0;
  border-radius: 24px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.16);
}

:deep(.project-modal .ant-modal-header) {
  margin-bottom: 0;
  padding: 24px;
  border-radius: 24px 24px 0 0;
  background: #fff;
}

:deep(.project-modal .ant-modal-title) {
  color: #262626;
  font-size: 24px;
  font-weight: 600;
  line-height: 32px;
}

:deep(.project-modal .ant-modal-body) {
  max-height: calc(100vh - 184px);
  overflow-y: auto;
  padding: 16px 48px 24px;
}

:deep(.project-modal .ant-modal-footer) {
  margin-top: 0;
  padding: 16px 48px;
  border-top: 1px solid #d9d9d9;
}

:deep(.project-modal .ant-modal-footer .ant-btn) {
  min-width: 72px;
  height: 32px;
  padding: 5px 16px;
  border-radius: 6px;
  font-size: 14px;
  line-height: 22px;
}

:deep(.project-modal-form .ant-form-item-label) {
  padding-bottom: 8px;
}

:deep(.project-modal-form .ant-form-item-label > label) {
  height: 22px;
  color: #262626;
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
}

:deep(.project-modal-form .ant-form-item-label > label.ant-form-item-required::before) {
  color: #fb363f;
}

.project-modal-label-hint {
  margin-left: 4px;
  color: #8c8c8c;
  font-weight: 400;
}

:deep(.project-modal-form .ant-input),
:deep(.project-modal-form .ant-input-affix-wrapper) {
  min-height: 40px;
  border-color: #d9d9d9;
  border-radius: 12px;
  color: #262626;
  font-size: 14px;
  line-height: 22px;
}

:deep(.project-modal-form .ant-input) {
  padding: 8px 12px;
}

:deep(.project-modal-form .ant-input-affix-wrapper) {
  padding: 0 8px 0 12px;
}

:deep(.project-modal-form .ant-input-affix-wrapper .ant-input) {
  min-height: 38px;
  padding: 8px 4px 8px 0;
  border: 0;
  border-radius: 0;
}

:deep(.project-modal-form .ant-input:hover),
:deep(.project-modal-form .ant-input-affix-wrapper:hover) {
  border-color: #3157e2;
}

:deep(.project-modal-form .ant-input:focus),
:deep(.project-modal-form .ant-input-affix-wrapper-focused) {
  border-color: #3157e2;
  box-shadow: 0 0 0 2px rgba(49, 87, 226, 0.15);
}

@media (max-width: 760px) {
  :deep(.project-modal .ant-modal-header) {
    padding: 20px 24px;
  }

  :deep(.project-modal .ant-modal-title) {
    font-size: 20px;
    line-height: 28px;
  }

  :deep(.project-modal .ant-modal-body) {
    padding: 16px 24px 24px;
  }

  :deep(.project-modal .ant-modal-footer) {
    padding: 16px 24px;
  }
}
</style>
