<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { Modal, message } from 'ant-design-vue'
import apiClient from '@/api/client'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

export interface ConfigOption {
  label: string
  value: string | number
}

export interface ConfigField {
  key: string
  label: string
  type?: 'text' | 'password' | 'number' | 'select' | 'textarea'
  required?: boolean
  placeholder?: string
  defaultValue?: string | number
  options?: ConfigOption[]
  table?: boolean
  width?: number
}

const props = defineProps<{
  title: string
  endpoint: string
  description: string
  fields: ConfigField[]
  emptyText?: string
  testable?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()
type Item = Record<string, unknown>

const loading = ref(false)
const saving = ref(false)
const items = ref<Item[]>([])
const visible = ref(false)
const editingID = ref<number | null>(null)
const form = reactive<Record<string, string | number>>({})

const columns = computed(() => [
  ...props.fields
    .filter((field) => field.table !== false && field.type !== 'password')
    .map((field) => ({
      title: field.label,
      dataIndex: field.key,
      key: field.key,
      width: field.width,
    })),
  { title: t('components.resourceTable.actions'), key: 'actions', width: 200 },
])

function resetForm() {
  editingID.value = null
  for (const field of props.fields) {
    form[field.key] = field.defaultValue ?? (field.type === 'number' ? 0 : '')
  }
}

async function load() {
  loading.value = true
  try {
    const response = await apiClient.get<{ items: Item[] }>(props.endpoint)
    items.value = response.items || []
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('components.resourceTable.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  resetForm()
  visible.value = true
}

function openEdit(item: Item) {
  resetForm()
  editingID.value = Number(item.id)
  for (const field of props.fields) {
    if (field.type === 'password') continue
    const value = item[field.key]
    if (value !== null && value !== undefined) {
      form[field.key] = field.type === 'number' || field.type === 'select' && typeof field.defaultValue === 'number'
        ? Number(value)
        : String(value)
    }
  }
  visible.value = true
}

function displayValue(field: ConfigField, value: unknown) {
  const option = field.options?.find((item) => String(item.value) === String(value))
  return option?.label ?? (value === null || value === undefined || value === '' ? '—' : String(value))
}

async function save() {
  const missing = props.fields.find((field) => field.required && !String(form[field.key] ?? '').trim())
  if (missing) {
    message.warning(t('components.resourceTable.fieldRequired', { field: missing.label }))
    return
  }
  const body: Record<string, string | number> = {}
  for (const field of props.fields) {
    const value = form[field.key]
    if (field.type === 'password' && editingID.value && !value) continue
    body[field.key] = value
  }
  saving.value = true
  try {
    if (editingID.value) {
      await apiClient.put(`${props.endpoint}/${editingID.value}`, body)
    } else {
      await apiClient.post(props.endpoint, body)
    }
    message.success(t('components.resourceTable.saveSuccess'))
    visible.value = false
    await load()
    emit('changed')
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('components.resourceTable.saveFailed'))
  } finally {
    saving.value = false
  }
}

function remove(item: Item) {
  Modal.confirm({
    title: t('components.resourceTable.deleteTitle', { name: String(item.name || t('components.resourceTable.defaultName')) }),
    content: t('components.resourceTable.deleteWarning'),
    okText: t('common.actions.delete'),
    okType: 'danger',
    cancelText: t('common.actions.cancel'),
    async onOk() {
      try {
        await apiClient.delete(`${props.endpoint}/${item.id}`)
        message.success(t('components.resourceTable.deleted'))
        await load()
        emit('changed')
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('components.resourceTable.deleteFailed'))
      }
    },
  })
}

const testingID = ref<number | null>(null)

async function testConnection(item: Item) {
  const id = Number(item.id)
  if (testingID.value === id) return
  testingID.value = id
  try {
    const res = await apiClient.post<{ ok: boolean; error?: string }>(`${props.endpoint}/${id}/test`)
    if (res && res.ok) {
      message.success(t('components.resourceTable.connectionSuccess', { name: String(item.name || t('components.resourceTable.defaultName')) }))
    } else {
      message.error(t('components.resourceTable.connectionFailed', { error: res?.error || t('components.resourceTable.unknownError') }))
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('components.resourceTable.testFailed'))
  } finally {
    testingID.value = null
  }
}

watch(() => props.fields, () => resetForm(), { deep: true })
onMounted(() => {
  resetForm()
  load()
})

defineExpose({ load, openCreate })
</script>

<template>
  <a-card :title="title">
    <template #extra>
      <a-space>
        <a-button @click="load">{{ t('common.actions.refresh') }}</a-button>
        <a-button type="primary" @click="openCreate">{{ t('components.resourceTable.add') }}</a-button>
      </a-space>
    </template>
    <a-alert :message="description" type="info" show-icon style="margin-bottom: 16px" />
    <a-table
      :columns="columns"
      :data-source="items"
      :loading="loading"
      row-key="id"
      :pagination="false"
      :locale="{ emptyText: emptyText || t('components.resourceTable.empty', { title }) }"
      :scroll="{ x: 720 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'actions'">
          <a-space>
            <a @click="openEdit(record)">{{ t('common.actions.edit') }}</a>
            <a
              v-if="testable"
              :class="{ 'testing-link': testingID === Number(record.id) }"
              @click="testConnection(record)"
            >{{ testingID === Number(record.id) ? t('components.resourceTable.testing') : t('components.resourceTable.testConnection') }}</a>
            <a class="danger-link" @click="remove(record)">{{ t('common.actions.delete') }}</a>
          </a-space>
        </template>
        <template v-else>
          {{ displayValue(fields.find((field) => field.key === column.key)!, record[column.key]) }}
        </template>
      </template>
    </a-table>
  </a-card>

  <a-modal
    v-model:open="visible"
    :title="editingID ? t('components.resourceTable.editTitle', { title }) : t('components.resourceTable.addTitle', { title })"
    :confirm-loading="saving"
    :ok-text="t('common.actions.save')"
    :cancel-text="t('common.actions.cancel')"
    @ok="save"
  >
    <a-form layout="vertical">
      <a-form-item
        v-for="field in fields"
        :key="field.key"
        :label="field.label"
        :required="field.required"
      >
        <a-input-number
          v-if="field.type === 'number'"
          v-model:value="form[field.key]"
          :min="0"
          style="width: 100%"
          :placeholder="field.placeholder"
        />
        <a-select
          v-else-if="field.type === 'select'"
          v-model:value="form[field.key]"
          :options="field.options"
          :placeholder="field.placeholder"
        />
        <a-textarea
          v-else-if="field.type === 'textarea'"
          v-model:value="form[field.key]"
          :rows="3"
          :placeholder="field.placeholder"
        />
        <a-input-password
          v-else-if="field.type === 'password'"
          v-model:value="form[field.key]"
          :placeholder="editingID ? t('components.resourceTable.passwordUnchanged') : field.placeholder"
        />
        <a-input
          v-else
          v-model:value="form[field.key]"
          :placeholder="field.placeholder"
        />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<style scoped>
.danger-link {
  color: #ff4d4f;
}
</style>
