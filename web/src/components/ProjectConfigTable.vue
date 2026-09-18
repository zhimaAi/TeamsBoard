<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Modal, message } from 'ant-design-vue'
import apiClient from '@/api/client'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

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
const title = computed(() => isGit.value ? t('components.projectTable.gitProject') : t('components.projectTable.dockerProject'))
const columns = computed(() => isGit.value
  ? [
      { title: t('components.projectTable.name'), dataIndex: 'name', key: 'name' },
      { title: 'SSH', dataIndex: 'ssh_profile_id', key: 'ssh_profile_id' },
      { title: t('components.projectTable.remoteDirectory'), dataIndex: 'remote_work_dir', key: 'remote_work_dir' },
      { title: t('components.resourceTable.actions'), key: 'actions', width: 140 },
    ]
  : [
      { title: t('components.projectTable.name'), dataIndex: 'name', key: 'name' },
      { title: 'SSH', dataIndex: 'ssh_profile_id', key: 'ssh_profile_id' },
      { title: t('components.projectTable.composeFile'), dataIndex: 'compose_file_path', key: 'compose_file_path' },
      { title: t('components.resourceTable.actions'), key: 'actions', width: 140 },
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
    message.error(error instanceof Error ? error.message : t('components.projectTable.loadFailed'))
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
    message.warning(t('components.projectTable.nameRequired'))
    return
  }
  if (isGit.value && (!form.ssh_profile_id || !form.remote_work_dir.trim())) {
    message.warning(t('components.projectTable.gitFieldsRequired'))
    return
  }
  if (!isGit.value && (!form.ssh_profile_id || !form.compose_file_path.trim())) {
    message.warning(t('components.projectTable.dockerFieldsRequired'))
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
    message.success(t('components.resourceTable.saveSuccess'))
    visible.value = false
    await load()
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('components.resourceTable.saveFailed'))
  } finally {
    saving.value = false
  }
}

function remove(item: Item) {
  Modal.confirm({
    title: t('components.resourceTable.deleteTitle', { name: item.name }),
    content: t('components.projectTable.deleteWarning'),
    okType: 'danger',
    async onOk() {
      await apiClient.delete(`${endpoint.value}/${item.id}`)
      message.success(t('components.resourceTable.deleted'))
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
        <a-button @click="load">{{ t('common.actions.refresh') }}</a-button>
        <a-button type="primary" @click="openCreate">{{ t('components.resourceTable.add') }}</a-button>
      </a-space>
    </template>
    <a-alert
      :message="isGit ? t('components.projectTable.gitDescription') : t('components.projectTable.dockerDescription')"
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
            <a @click="openEdit(record)">{{ t('common.actions.edit') }}</a>
            <a style="color: #ff4d4f" @click="remove(record)">{{ t('common.actions.delete') }}</a>
          </a-space>
        </template>
      </template>
    </a-table>
  </a-card>

  <a-modal
    v-model:open="visible"
    :title="editingID ? t('components.resourceTable.editTitle', { title }) : t('components.resourceTable.addTitle', { title })"
    :confirm-loading="saving"
    @ok="save"
  >
    <a-form layout="vertical">
      <a-form-item :label="t('components.projectTable.name')" required>
        <a-input v-model:value="form.name" />
      </a-form-item>
      <template v-if="isGit">
        <a-form-item :label="t('components.projectTable.sshProfile')" required>
          <a-select v-model:value="form.ssh_profile_id" :placeholder="t('components.projectTable.chooseSsh')">
            <a-select-option v-for="profile in sshProfiles" :key="profile.id" :value="Number(profile.id)">
              {{ profile.name }}（{{ profile.username }}@{{ profile.host }}:{{ profile.port }}）
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item :label="t('components.projectTable.remoteDirectory')" required>
          <a-input v-model:value="form.remote_work_dir" placeholder="/var/www/project" />
        </a-form-item>
      </template>
      <template v-else>
        <a-form-item :label="t('components.projectTable.sshProfile')" required>
          <a-select v-model:value="form.ssh_profile_id" :placeholder="t('components.projectTable.chooseSsh')">
            <a-select-option v-for="profile in sshProfiles" :key="profile.id" :value="Number(profile.id)">
              {{ profile.name }}（{{ profile.username }}@{{ profile.host }}:{{ profile.port }}）
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item :label="t('components.projectTable.composeFilePath')" required>
          <a-input v-model:value="form.compose_file_path" placeholder="/var/www/app/compose.yml" />
        </a-form-item>
      </template>
    </a-form>
  </a-modal>
</template>
