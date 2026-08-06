<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Modal, message } from 'ant-design-vue'
import apiClient from '@/api/client'

const props = defineProps<{ kind: 'git' | 'docker' }>()

type Item = Record<string, string | number>
const loading = ref(false)
const saving = ref(false)
const items = ref<Item[]>([])
const sshProfiles = ref<Item[]>([])
const visible = ref(false)
const editingID = ref<number | null>(null)
const form = reactive({
  name: '',
  ssh_profile_id: 0,
  remote_work_dir: '',
  compose_file_path: '',
})

const isGit = computed(() => props.kind === 'git')
const endpoint = computed(() => isGit.value ? '/config/git-projects' : '/config/docker-projects')
const title = computed(() => isGit.value ? 'Git 项目' : 'Docker Compose 项目')
const columns = computed(() => isGit.value
  ? [
      { title: '名称', dataIndex: 'name', key: 'name' },
      { title: 'SSH', dataIndex: 'ssh_profile_id', key: 'ssh_profile_id' },
      { title: '远程工作目录', dataIndex: 'remote_work_dir', key: 'remote_work_dir' },
      { title: '操作', key: 'actions', width: 140 },
    ]
  : [
      { title: '名称', dataIndex: 'name', key: 'name' },
      { title: 'SSH', dataIndex: 'ssh_profile_id', key: 'ssh_profile_id' },
      { title: 'Compose 文件', dataIndex: 'compose_file_path', key: 'compose_file_path' },
      { title: '操作', key: 'actions', width: 140 },
    ])

async function load() {
  loading.value = true
  try {
    const requests: Promise<unknown>[] = [
      apiClient.get<{ items: Item[] }>(endpoint.value).then((response) => {
        items.value = response.items || []
      }),
    ]
    if (isGit.value || props.kind === 'docker') {
      requests.push(apiClient.get<{ items: Item[] }>('/config/ssh-profiles').then((response) => {
        sshProfiles.value = response.items || []
      }))
    }
    await Promise.all(requests)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载失败')
  } finally {
    loading.value = false
  }
}

function resetForm() {
  editingID.value = null
  form.name = ''
  form.ssh_profile_id = 0
  form.remote_work_dir = ''
  form.compose_file_path = ''
}

function openCreate() {
  resetForm()
  visible.value = true
}

function openEdit(item: Item) {
  resetForm()
  editingID.value = Number(item.id)
  form.name = String(item.name || '')
  form.ssh_profile_id = Number(item.ssh_profile_id || 0)
  form.remote_work_dir = String(item.remote_work_dir || '')
  form.compose_file_path = String(item.compose_file_path || '')
  visible.value = true
}

async function save() {
  if (!form.name.trim()) {
    message.warning('请输入名称')
    return
  }
  if (isGit.value && (!form.ssh_profile_id || !form.remote_work_dir.trim())) {
    message.warning('请选择 SSH 配置并填写远程工作目录')
    return
  }
  if (!isGit.value && (!form.ssh_profile_id || !form.compose_file_path.trim())) {
    message.warning('请选择 SSH 配置并填写 Compose 文件路径')
    return
  }
  const body = isGit.value
    ? {
        name: form.name,
        ssh_profile_id: form.ssh_profile_id,
        remote_work_dir: form.remote_work_dir,
      }
    : { name: form.name, ssh_profile_id: form.ssh_profile_id, compose_file_path: form.compose_file_path }
  saving.value = true
  try {
    if (editingID.value) {
      await apiClient.put(`${endpoint.value}/${editingID.value}`, body)
    } else {
      await apiClient.post(endpoint.value, body)
    }
    message.success('保存成功')
    visible.value = false
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    saving.value = false
  }
}

function remove(item: Item) {
  Modal.confirm({
    title: `删除“${item.name}”`,
    content: '只删除 GoTeams 中的配置，不会删除远程代码或 Compose 文件。',
    okType: 'danger',
    async onOk() {
      await apiClient.delete(`${endpoint.value}/${item.id}`)
      message.success('已删除')
      await load()
    },
  })
}

onMounted(load)
</script>

<template>
  <a-card :title="title">
    <template #extra>
      <a-space>
        <a-button @click="load">刷新</a-button>
        <a-button type="primary" @click="openCreate">新增</a-button>
      </a-space>
    </template>
    <a-alert
      :message="isGit ? 'Git 命令会通过 SSH 在配置的远程工作目录中执行。' : 'Docker 命令会通过 SSH 在远程服务器上以 docker compose -f 指定文件执行。'"
      type="info"
      show-icon
      style="margin-bottom: 16px"
    />
    <a-table :columns="columns" :data-source="items" :loading="loading" row-key="id" :pagination="false">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'ssh_profile_id'">
          {{ sshProfiles.find(item => Number(item.id) === Number(record.ssh_profile_id))?.name || '-' }}
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a @click="openEdit(record)">编辑</a>
            <a style="color: #ff4d4f" @click="remove(record)">删除</a>
          </a-space>
        </template>
      </template>
    </a-table>
  </a-card>

  <a-modal
    v-model:open="visible"
    :title="editingID ? `编辑${title}` : `新增${title}`"
    :confirm-loading="saving"
    @ok="save"
  >
    <a-form layout="vertical">
      <a-form-item label="名称" required>
        <a-input v-model:value="form.name" />
      </a-form-item>
      <template v-if="isGit">
        <a-form-item label="SSH 配置" required>
          <a-select v-model:value="form.ssh_profile_id" placeholder="请选择 SSH 配置">
            <a-select-option v-for="profile in sshProfiles" :key="profile.id" :value="Number(profile.id)">
              {{ profile.name }}（{{ profile.username }}@{{ profile.host }}:{{ profile.port }}）
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="远程工作目录" required>
          <a-input v-model:value="form.remote_work_dir" placeholder="/var/www/project" />
        </a-form-item>
      </template>
      <template v-else>
        <a-form-item label="SSH 配置" required>
          <a-select v-model:value="form.ssh_profile_id" placeholder="请选择 SSH 配置">
            <a-select-option v-for="profile in sshProfiles" :key="profile.id" :value="Number(profile.id)">
              {{ profile.name }}（{{ profile.username }}@{{ profile.host }}:{{ profile.port }}）
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="Compose 文件路径" required>
          <a-input v-model:value="form.compose_file_path" placeholder="/var/www/app/compose.yml" />
        </a-form-item>
      </template>
    </a-form>
  </a-modal>
</template>
