<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import apiClient from '@/api/client'
import ConfigResourceTable, { type ConfigField } from '@/components/ConfigResourceTable.vue'
import { useAppI18n } from '@/i18n'

type Item = Record<string, unknown>
const sshProfiles = ref<Item[]>([])
const { t } = useAppI18n()

const fields = computed<ConfigField[]>(() => [
  { key: 'name', label: t('settings.name'), required: true, placeholder: t('settings.exampleDatabase') },
  {
    key: 'db_type',
    label: t('settings.type'),
    type: 'select',
    required: true,
    defaultValue: 'mysql',
    options: [
      { label: 'MySQL', value: 'mysql' },
      { label: 'PostgreSQL', value: 'postgresql' },
    ],
  },
  { key: 'host', label: t('settings.host'), placeholder: '127.0.0.1' },
  { key: 'port', label: t('settings.port'), type: 'number', defaultValue: 3306, width: 90 },
  { key: 'database_name', label: t('settings.database') },
  { key: 'username', label: t('settings.username') },
  { key: 'password', label: t('settings.password'), type: 'password', table: false },
  {
    key: 'ssh_profile_id',
    label: t('settings.sshTunnel'),
    type: 'select',
    defaultValue: 0,
    options: [
      { label: t('settings.none'), value: 0 },
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
    :title="t('settings.databaseConnections')"
    endpoint="/config/database-profiles"
    :description="t('settings.databaseDescription')"
    :fields="fields"
    :testable="true"
  />
</template>
