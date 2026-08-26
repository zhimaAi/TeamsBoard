<template>
  <a-modal
    :open="open"
    :title="pipeline ? '编辑流水线' : '新建流水线'"
    width="640px"
    :confirm-loading="saving"
    @update:open="emit('update:open', $event)"
    @ok="save"
  >
    <a-form layout="vertical">
      <a-form-item
        label="流水线名称（最多20个字）"
        required
      >
        <a-input
          v-model:value="form.name"
          :maxlength="20"
          show-count
          placeholder="请输入流水线名称"
        />
      </a-form-item>
      <a-form-item label="流水线头像">
        <div class="avatar-options">
          <button
            v-for="avatar in PIPELINE_AVATARS"
            :key="avatar"
            type="button"
            :class="{ active: form.avatar === avatar }"
            @click="form.avatar = avatar"
          >
            <img
              :src="avatar"
              alt=""
            />
          </button>
        </div>
      </a-form-item>
      <a-form-item label="简介（最多200个字）">
        <a-textarea
          v-model:value="form.description"
          :maxlength="200"
          show-count
          :rows="3"
          placeholder="描述流水线的用途与协作流程"
        />
      </a-form-item>
    </a-form>
    <template #footer>
      <a-button
        v-if="pipeline"
        danger
        class="delete-pipeline"
        @click="remove"
      >
        <DeleteOutlined />删除
      </a-button>
      <a-button @click="emit('update:open', false)">取消</a-button>
      <a-button
        type="primary"
        :loading="saving"
        @click="save"
      >
        {{ pipeline ? '保存修改' : '确认创建' }}
      </a-button>
    </template>
  </a-modal>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { DeleteOutlined } from '@ant-design/icons-vue'
import { message, Modal } from 'ant-design-vue'
import apiClient from '@/api/client'
import type { Pipeline } from '@/types/pipeline'
import { PIPELINE_AVATARS, type DeleteResponse, type PipelineInput } from './agentPipeline'

const props = defineProps<{
  open: boolean
  pipeline?: Pipeline
}>()

const emit = defineEmits<{
  created: [pipeline: Pipeline]
  deleted: []
  updated: [pipeline: Pipeline]
  'update:open': [open: boolean]
}>()

const saving = ref(false)
const form = reactive<PipelineInput>({
  name: '',
  description: '',
  avatar: PIPELINE_AVATARS[0],
})

watch(
  () => props.open,
  (open) => {
    if (!open) return
    form.name = props.pipeline?.name || ''
    form.description = props.pipeline?.description || ''
    form.avatar = props.pipeline?.avatar || PIPELINE_AVATARS[0]
  },
)

async function save() {
  const name = form.name.trim()
  if (!name) return message.warning('请输入流水线名称')
  if (name.length > 20) return message.warning('流水线名称最多20个字')
  if (form.description.length > 200) return message.warning('流水线简介最多200个字')

  saving.value = true
  try {
    const payload: PipelineInput = {
      name,
      description: form.description.trim(),
      avatar: form.avatar,
    }
    if (props.pipeline) {
      const updated = await apiClient.put<Pipeline>(`/pipelines/${props.pipeline.uuid}`, payload)
      emit('update:open', false)
      emit('updated', updated)
      message.success('流水线已更新')
    } else {
      const created = await apiClient.post<Pipeline>('/pipelines', payload)
      emit('update:open', false)
      emit('created', created)
      message.success('流水线已创建')
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '流水线保存失败')
  } finally {
    saving.value = false
  }
}

function remove() {
  if (!props.pipeline) return
  Modal.confirm({
    title: `删除流水线“${props.pipeline.name}”？`,
    content: '不会影响已经创建任务中的执行快照。',
    okType: 'danger',
    onOk: async () => {
      try {
        await apiClient.delete<DeleteResponse>(`/pipelines/${props.pipeline!.uuid}`)
        emit('update:open', false)
        emit('deleted')
      } catch (error) {
        message.error(error instanceof Error ? error.message : '流水线删除失败')
        throw error
      }
    },
  })
}
</script>

<style scoped>
.avatar-options {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.avatar-options button {
  width: 48px;
  height: 48px;
  padding: 2px;
  border: 2px solid transparent;
  border-radius: 50%;
  background: #fff;
  cursor: pointer;
}

.avatar-options button.active {
  border-color: #1677ff;
}

.avatar-options button:focus-visible {
  outline: 2px solid #1677ff;
  outline-offset: 2px;
}

.avatar-options img {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  object-fit: cover;
}

.delete-pipeline {
  margin-right: auto;
}
</style>
