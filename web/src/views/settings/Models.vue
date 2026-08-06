<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import apiClient from '@/api/client'
import ConfigResourceTable, { type ConfigField } from '@/components/ConfigResourceTable.vue'

type Item = Record<string, unknown>
const providers = ref<Item[]>([])
const providerTable = ref<InstanceType<typeof ConfigResourceTable> | null>(null)

const providerFields: ConfigField[] = [
  { key: 'name', label: '名称', required: true, placeholder: '例如：OpenAI' },
  {
    key: 'provider_type',
    label: '类型',
    type: 'select',
    required: true,
    defaultValue: 'openai',
    options: [
      { label: 'OpenAI 兼容', value: 'openai' },
      { label: 'Anthropic', value: 'anthropic' },
      { label: 'Ollama', value: 'ollama' },
      { label: '自定义', value: 'custom' },
    ],
  },
  { key: 'api_base_url', label: 'API 地址', placeholder: 'https://api.openai.com/v1' },
  { key: 'api_key', label: 'API Key', type: 'password', table: false },
]

const profileFields = computed<ConfigField[]>(() => [
  { key: 'name', label: '配置名称', required: true, placeholder: '例如：默认编码模型' },
  {
    key: 'provider_id',
    label: '服务商',
    type: 'select',
    required: true,
    options: providers.value.map((item) => ({ label: String(item.name), value: Number(item.id) })),
  },
  { key: 'model_name', label: '模型名称', required: true, placeholder: 'gpt-5' },
  { key: 'temperature', label: 'Temperature', type: 'number', defaultValue: 0.7 },
  { key: 'max_tokens', label: '最大 Token', type: 'number', defaultValue: 4096 },
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
      title="模型服务商"
      endpoint="/config/model-providers"
      description="配置 OpenAI 兼容、Anthropic、Ollama 或自定义模型接口。密钥保存在系统安全存储中。"
      :fields="providerFields"
      @changed="loadProviders"
    />
    <ConfigResourceTable
      title="模型配置"
      endpoint="/config/model-profiles"
      description="选择服务商并配置具体模型参数，CLI 配置可引用这里的模型。"
      empty-text="请先新增模型服务商，再创建模型配置"
      :fields="profileFields"
    />
  </a-space>
</template>
