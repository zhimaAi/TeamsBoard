<script setup lang="ts">
import { computed } from 'vue'
import ConfigResourceTable, { type ConfigField } from '@/components/ConfigResourceTable.vue'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()
const fields = computed<ConfigField[]>(() => [
  { key: 'name', label: t('settings.name'), required: true, placeholder: t('settings.exampleServer') },
  { key: 'host', label: t('settings.host'), required: true, placeholder: '192.168.1.10' },
  { key: 'port', label: t('settings.port'), type: 'number', defaultValue: 22, width: 90 },
  { key: 'username', label: t('settings.username'), placeholder: 'root' },
  { key: 'password', label: t('settings.password'), type: 'password', table: false },
  // { key: 'key_path', label: 'Private key path', placeholder: 'C:\Users\me\.ssh\id_ed25519' },
])
</script>

<template>
  <ConfigResourceTable
    :title="t('settings.sshConfig')"
    endpoint="/config/ssh-profiles"
    :description="t('settings.sshDescription')"
    :fields="fields"
    :testable="true"
  />
</template>
