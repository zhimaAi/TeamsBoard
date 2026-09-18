<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import apiClient from '@/api/client'
import ConfigResourceTable, { type ConfigField } from '@/components/ConfigResourceTable.vue'
import { useAppI18n } from '@/i18n'

type Item = Record<string, unknown>
const providers = ref<Item[]>([])
const providerTable = ref<InstanceType<typeof ConfigResourceTable> | null>(null)
const { t } = useAppI18n()

const providerFields = computed<ConfigField[]>(() => [
  { key: 'name', label: t('settings.name'), required: true, placeholder: t('settings.exampleOpenAI') },
  {
    key: 'provider_type',
    label: t('settings.type'),
    type: 'select',
    required: true,
    defaultValue: 'openai',
    options: [
      { label: t('settings.openAICompatible'), value: 'openai' },
      { label: 'Anthropic', value: 'anthropic' },
      { label: 'Ollama', value: 'ollama' },
      { label: t('settings.custom'), value: 'custom' },
    ],
  },
  { key: 'api_base_url', label: t('settings.apiAddress'), placeholder: 'https://api.openai.com/v1' },
  { key: 'api_key', label: 'API Key', type: 'password', table: false },
])

const profileFields = computed<ConfigField[]>(() => [
  { key: 'name', label: t('settings.profileName'), required: true, placeholder: t('settings.exampleModelProfile') },
  {
    key: 'provider_id',
    label: t('settings.provider'),
    type: 'select',
    required: true,
    options: providers.value.map((item) => ({ label: String(item.name), value: Number(item.id) })),
  },
  { key: 'model_name', label: t('settings.modelName'), required: true, placeholder: 'gpt-5' },
  { key: 'temperature', label: 'Temperature', type: 'number', defaultValue: 0.7 },
  { key: 'max_tokens', label: t('settings.maxTokens'), type: 'number', defaultValue: 4096 },
])

async function loadProviders() {
  const result = await apiClient.get<{ items: Item[] }>('/config/model-providers').catch(() => ({ items: [] }))
  providers.value = result.items || []
}

onMounted(loadProviders)
</script>

<template>
  <a-space direction="vertical" :size="16" style="width: 100%">
    <ConfigResourceTable
      ref="providerTable"
      :title="t('settings.modelProviders')"
      endpoint="/config/model-providers"
      :description="t('settings.modelProviderDescription')"
      :fields="providerFields"
      @changed="loadProviders"
    />
    <ConfigResourceTable
      :title="t('settings.modelConfigs')"
      endpoint="/config/model-profiles"
      :description="t('settings.modelConfigDescription')"
      :empty-text="t('settings.modelProviderRequired')"
      :fields="profileFields"
    />
  </a-space>
</template>
