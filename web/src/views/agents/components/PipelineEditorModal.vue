<template>
  <a-modal
    :open="open"
    class="pipeline-editor-modal"
	:title="modalTitle"
    width="730px"
    centered
    @update:open="handleOpenChange"
  >

    <button
      class="pipeline-icon-preview"
      type="button"
      :aria-label="t('agents.uploadPipelineIcon')"
      @click="openFilePicker"
    >
      <img
        :src="previewIcon"
        alt=""
      />
      <span class="pipeline-icon-preview__camera">
        <img
          :src="projectModalCameraIcon"
          alt=""
          aria-hidden="true"
        />
      </span>
    </button>
    <p class="pipeline-icon-hint">{{ t('agents.iconHint') }}</p>
    <input
      ref="fileInput"
      class="pipeline-icon-file-input"
      type="file"
      :accept="ICON_FILE_ACCEPT"
      @change="handleFileChange"
    />

    <a-form
      class="pipeline-editor-form"
      layout="vertical"
    >
      <a-form-item
        class="pipeline-icon-form-item"
		:label="avatarLabel"
      >
        <div class="avatar-options">
          <button
            v-for="avatar in PIPELINE_AVATARS"
            :key="avatar"
            type="button"
            :class="{ active: !iconFile && form.avatar === avatar }"
            :aria-pressed="!iconFile && form.avatar === avatar"
			:aria-label="avatarSelectLabel"
            @click="selectPresetAvatar(avatar)"
          >
            <img
              :src="avatar"
              alt=""
            />
          </button>
        </div>
      </a-form-item>

      <a-form-item class="pipeline-editor-form-item">
        <template #label>
		  <span class="pipeline-modal-label">{{ nameLabel }}</span>
          <span class="pipeline-modal-label-hint">{{ t('agents.nameLimit') }}</span>
        </template>
        <a-input
          v-model:value="form.name"
          :maxlength="20"
          :show-count="false"
		  :placeholder="namePlaceholder"
        />
      </a-form-item>

      <a-form-item class="pipeline-editor-form-item pipeline-intro-form-item">
        <template #label>
          <span class="pipeline-modal-label">{{ t('agents.intro') }}</span>
          <span class="pipeline-modal-label-hint pipeline-modal-label-hint--flush">{{ t('agents.introLimit') }}</span>
        </template>
        <a-textarea
          v-model:value="form.description"
          :maxlength="200"
          :show-count="false"
		  :placeholder="introPlaceholder"
        />
      </a-form-item>
    </a-form>

    <template #footer>
      <div class="pipeline-modal-footer-actions">
        <a-button @click="handleOpenChange(false)">{{ t('agents.cancel') }}</a-button>
        <a-button
          type="primary"
          :loading="saving"
          @click="save"
        >
		  {{ editingResource ? t('agents.saveChanges') : t('agents.confirmCreate') }}
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
import { ICON_FILE_ACCEPT, useIconFile } from '@/composables/useIconFile'
import type { ExpertGroup, Pipeline } from '@/types/pipeline'
import { PIPELINE_AVATARS, type PipelineInput } from './agentPipeline'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

const props = withDefaults(defineProps<{
  open: boolean
  kind?: 'pipeline' | 'expert_group'
  pipeline?: Pipeline
  expertGroup?: ExpertGroup
}>(), { kind: 'pipeline' })

const emit = defineEmits<{
  created: [pipeline: Pipeline]
  updated: [pipeline: Pipeline]
	'expert-created': [group: ExpertGroup]
	'expert-updated': [group: ExpertGroup]
  'update:open': [open: boolean]
}>()

const saving = ref(false)
const fileInput = ref<HTMLInputElement>()
const form = reactive<PipelineInput>({
  name: '',
  description: '',
  avatar: PIPELINE_AVATARS[0],
})
const { file: iconFile, previewUrl, selectFile, reset: resetIconFile } = useIconFile()
// 设计稿约束：流水线图标大小不超过 100KB，比共享图标校验（2MB）更严格，仅作用于本弹窗
const MAX_PIPELINE_ICON_FILE_SIZE = 100 * 1024

const isExpertGroup = computed(() => props.kind === 'expert_group')
const editingResource = computed(() => isExpertGroup.value ? props.expertGroup : props.pipeline)
const modalTitle = computed(() => isExpertGroup.value
	? editingResource.value ? t('expertGroups.editGroup') : t('expertGroups.newGroup')
	: editingResource.value ? t('agents.editPipeline') : t('agents.newPipeline'))
const avatarLabel = computed(() => isExpertGroup.value ? t('expertGroups.defaultAvatar') : t('agents.defaultPipelineAvatar'))
const avatarSelectLabel = computed(() => isExpertGroup.value ? t('expertGroups.selectAvatar') : t('agents.selectPipelineAvatar'))
const nameLabel = computed(() => isExpertGroup.value ? t('expertGroups.name') : t('agents.pipelineName'))
const namePlaceholder = computed(() => isExpertGroup.value ? t('expertGroups.namePlaceholder') : t('agents.enter'))
const introPlaceholder = computed(() => isExpertGroup.value ? t('expertGroups.descriptionPlaceholder') : t('agents.introPlaceholder'))
const previewIcon = computed(() => previewUrl.value || form.avatar || PIPELINE_AVATARS[0])

watch(
  () => props.open,
  (open) => {
    if (!open) return
    resetIconFile()
	const resource = editingResource.value
	form.name = resource?.name || ''
	form.description = resource?.description || ''
	form.avatar = resource?.avatar || PIPELINE_AVATARS[0]
  },
)

function openFilePicker() {
  fileInput.value?.click()
}

function handleFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const nextFile = input.files?.[0]
  input.value = ''
  if (!nextFile) return

  if (nextFile.size > MAX_PIPELINE_ICON_FILE_SIZE) {
    message.warning(t('agents.imageTooLarge'))
    return
  }

  try {
    selectFile(nextFile)
  } catch (error) {
    message.warning(error instanceof Error ? error.message : t('agents.selectIconFailed'))
  }
}

function selectPresetAvatar(avatar: string) {
  resetIconFile()
  form.avatar = avatar
}

function buildPipelineFormData(payload: PipelineInput) {
  const formData = new FormData()
  formData.append('name', payload.name)
  formData.append('description', payload.description)
  formData.append('avatar', payload.avatar)
  if (iconFile.value) formData.append('avatar_file', iconFile.value)
  return formData
}

async function save() {
  const name = form.name.trim()
  if (!name) return message.warning(t('agents.enterPipelineName'))
  if (name.length > 20) return message.warning(t('agents.pipelineNameTooLong'))
  if (form.description.length > 200) return message.warning(t('agents.introTooLong'))

  saving.value = true
  try {
    const payload: PipelineInput = {
      name,
      description: form.description.trim(),
      avatar: form.avatar,
    }
    const body = iconFile.value ? buildPipelineFormData(payload) : payload
	if (isExpertGroup.value) {
	  if (props.expertGroup) {
		const updated = await apiClient.put<ExpertGroup>(`/expert-groups/${encodeURIComponent(props.expertGroup.uuid)}`, body)
		emit('update:open', false)
		emit('expert-updated', updated)
		message.success(t('expertGroups.saved'))
	  } else {
		const created = await apiClient.post<ExpertGroup>('/expert-groups', body)
		emit('update:open', false)
		emit('expert-created', created)
		message.success(t('expertGroups.saved'))
	  }
	} else if (props.pipeline) {
	  const updated = await apiClient.put<Pipeline>(`/pipelines/${encodeURIComponent(props.pipeline.uuid)}`, body)
      emit('update:open', false)
      emit('updated', updated)
      message.success(t('agents.pipelineUpdated'))
    } else {
      const created = await apiClient.post<Pipeline>('/pipelines', body)
      emit('update:open', false)
      emit('created', created)
      message.success(t('agents.pipelineCreated'))
    }
  } catch (error) {
	message.error(error instanceof Error ? error.message : isExpertGroup.value ? t('expertGroups.saveFailed') : t('agents.pipelineSaveFailed'))
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
.pipeline-icon-preview {
  position: relative;
  display: flex;
  width: 60px;
  height: 60px;
  align-items: center;
  justify-content: center;
  margin: 0 auto;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: #f2f3f5;
  cursor: pointer;
}

.pipeline-icon-preview > img {
  display: block;
  width: 60px;
  height: 60px;
  border-radius: 50%;
  object-fit: cover;
}

.pipeline-icon-preview__camera {
  position: absolute;
  right: -3px;
  bottom: -4px;
  display: flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border: 0.75px solid #fff;
  border-radius: 50%;
  background: #262626;
  box-shadow: 0 2px 4px rgba(221, 221, 221, 0.12);
}

.pipeline-icon-preview__camera img {
  display: block;
  width: 12px;
  height: 12px;
}

.pipeline-icon-preview:focus-visible,
.avatar-options button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.pipeline-icon-hint {
  margin: 10px 0 0;
  color: rgba(0, 0, 0, 0.45);
  font-size: 14px;
  line-height: 22px;
  text-align: center;
}

.pipeline-icon-file-input {
  position: fixed;
  width: 1px;
  height: 1px;
  overflow: hidden;
  opacity: 0;
  pointer-events: none;
}

.pipeline-editor-form {
  margin-top: 24px;
}

.pipeline-icon-form-item,
.pipeline-editor-form-item {
  margin-bottom: 20px;
}

.pipeline-intro-form-item {
  margin-bottom: 0;
}

.avatar-options {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.avatar-options button {
  display: flex;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  padding: 1px;
  border: 2px solid transparent;
  border-radius: 50%;
  background: #fff;
  cursor: pointer;
}

.avatar-options button.active {
  border-color: #3157e2;
}

.avatar-options img {
  display: block;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  object-fit: cover;
}

.pipeline-modal-footer-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

:deep(.pipeline-editor-modal) {
  max-width: calc(100vw - 32px);
  padding-bottom: 0;
}

:deep(.pipeline-editor-modal .ant-modal-content) {
  overflow: hidden;
  padding: 0;
  border-radius: 24px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.16);
}

:deep(.pipeline-editor-modal .ant-modal-header) {
  margin-bottom: 0;
  padding: 24px;
  border-radius: 24px 24px 0 0;
  background: #fff;
}

:deep(.pipeline-editor-modal .ant-modal-title) {
  color: #262626;
  font-size: 24px;
  font-weight: 600;
  line-height: 32px;
}

:deep(.pipeline-editor-modal .ant-modal-body) {
  max-height: calc(100vh - 184px);
  overflow-y: auto;
  padding: 16px 48px 24px;
}

:deep(.pipeline-editor-modal .ant-modal-footer) {
  margin-top: 0;
  padding: 16px 48px;
  border-top: 1px solid #d9d9d9;
}

:deep(.pipeline-editor-modal .ant-modal-footer .ant-btn) {
  min-width: 65px;
  height: 32px;
  padding: 5px 16px;
  border-radius: 6px;
  font-size: 14px;
  line-height: 22px;
}

:deep(.pipeline-editor-form .ant-form-item-label) {
  padding-bottom: 8px;
}

:deep(.pipeline-editor-form .ant-form-item-label > label) {
  height: 22px;
  color: #262626;
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
}

.pipeline-modal-label-hint {
  margin-left: 4px;
  color: #8c8c8c;
  font-weight: 400;
}

.pipeline-modal-label-hint--flush {
  margin-left: 0;
}

:deep(.pipeline-editor-form .ant-input) {
  min-height: 40px;
  padding: 8px 12px;
  border-color: #d9d9d9 !important;
  border-radius: 12px !important;
  color: #262626;
  font-size: 14px;
  line-height: 22px;
}

:deep(.pipeline-editor-form textarea.ant-input) {
  height: 128px;
  min-height: 128px;
  resize: none;
}

:deep(.pipeline-editor-form .ant-input:hover) {
  border-color: #3157e2 !important;
}

:deep(.pipeline-editor-form .ant-input:focus),
:deep(.pipeline-editor-form .ant-input-focused) {
  border-color: #3157e2 !important;
  box-shadow: 0 0 0 2px rgba(49, 87, 226, 0.15) !important;
}

@media (max-width: 760px) {
  :deep(.pipeline-editor-modal .ant-modal-header) {
    padding: 20px 24px;
  }

  :deep(.pipeline-editor-modal .ant-modal-title) {
    font-size: 20px;
    line-height: 28px;
  }

  :deep(.pipeline-editor-modal .ant-modal-body) {
    padding: 16px 24px 24px;
  }

  :deep(.pipeline-editor-modal .ant-modal-footer) {
    padding: 16px 24px;
  }
}
</style>
