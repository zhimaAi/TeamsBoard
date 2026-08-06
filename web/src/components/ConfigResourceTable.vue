<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { Modal, message } from 'ant-design-vue'
import apiClient from '@/api/client'

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
  { title: '操作', key: 'actions', width: 200 },
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
    message.error(error instanceof Error ? error.message : '加载配置失败')
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
    message.warning(`请填写${missing.label}`)
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
    message.success('保存成功')
    visible.value = false
    await load()
    emit('changed')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    saving.value = false
  }
}

function remove(item: Item) {
  Modal.confirm({
    title: `删除“${String(item.name || '该配置')}”`,
    content: '删除后依赖此项的配置可能无法正常工作。',
    okText: '删除',
    okType: 'danger',
    cancelText: '取消',
    async onOk() {
      try {
        await apiClient.delete(`${props.endpoint}/${item.id}`)
        message.success('已删除')
        await load()
        emit('changed')
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除失败')
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
      message.success(`“${String(item.name || '该配置')}”连接成功`)
    } else {
      message.error(`连接失败：${res?.error || '未知错误'}`)
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '测试连接失败')
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
        <a-button @click="load">刷新</a-button>
        <a-button type="primary" @click="openCreate">新增</a-button>
      </a-space>
    </template>
    <a-alert :message="description" type="info" show-icon style="margin-bottom: 16px" />
    <a-table
      :columns="columns"
      :data-source="items"
      :loading="loading"
      row-key="id"
      :pagination="false"
      :locale="{ emptyText: emptyText || `暂无${title}，点击右上角“新增”开始配置` }"
      :scroll="{ x: 720 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'actions'">
          <a-space>
            <a @click="openEdit(record)">编辑</a>
            <a
              v-if="testable"
              :class="{ 'testing-link': testingID === Number(record.id) }"
              @click="testConnection(record)"
            >{{ testingID === Number(record.id) ? '测试中…' : '测试链接' }}</a>
            <a class="danger-link" @click="remove(record)">删除</a>
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
    :title="editingID ? `编辑${title}` : `新增${title}`"
    :confirm-loading="saving"
    ok-text="保存"
    cancel-text="取消"
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
          :placeholder="editingID ? '留空表示不修改' : field.placeholder"
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
