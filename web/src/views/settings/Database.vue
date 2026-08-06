<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import apiClient from '@/api/client'
import ConfigResourceTable, { type ConfigField } from '@/components/ConfigResourceTable.vue'

type Item = Record<string, unknown>
const sshProfiles = ref<Item[]>([])

const fields = computed<ConfigField[]>(() => [
  { key: 'name', label: '名称', required: true, placeholder: '例如：开发库' },
  {
    key: 'db_type',
    label: '类型',
    type: 'select',
    required: true,
    defaultValue: 'mysql',
    options: [
      { label: 'MySQL', value: 'mysql' },
      { label: 'PostgreSQL', value: 'postgresql' },
    ],
  },
  { key: 'host', label: '主机', placeholder: '127.0.0.1' },
  { key: 'port', label: '端口', type: 'number', defaultValue: 3306, width: 90 },
  { key: 'database_name', label: '数据库' },
  { key: 'username', label: '用户名' },
  { key: 'password', label: '密码', type: 'password', table: false },
  {
    key: 'ssh_profile_id',
    label: 'SSH 隧道',
    type: 'select',
    defaultValue: 0,
    options: [
      { label: '不使用', value: 0 },
      ...sshProfiles.value.map((item) => ({ label: String(item.name), value: Number(item.id) })),
    ],
  },
])

onMounted(async () => {
  const result = await apiClient.get<{ items: Item[] }>('/config/ssh-profiles').catch(() => ({ items: [] }))
  sshProfiles.value = result.items || []
})
</script>

<template>
  <ConfigResourceTable
    title="数据库连接"
    endpoint="/config/database-profiles"
    description="管理 MySQL / PostgreSQL 连接，可选用已配置的 SSH 隧道。密码只保存在系统安全存储中。"
    :fields="fields"
    :testable="true"
  />
</template>
