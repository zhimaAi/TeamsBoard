<template>
  <a-dropdown
    :open="open"
    :trigger="['click']"
    @update:open="open = $event"
  >
    <a-button
      type="primary"
      size="small"
      class="add-button"
      aria-haspopup="menu"
      :aria-expanded="open"
    >
      <template #icon><PlusOutlined /></template>
      {{ label || t('agents.add') }}
    </a-button>
    <template #overlay>
      <a-menu @click="handleClick">
        <a-menu-item key="create">
          <img
            class="add-menu-icon add-menu-icon--new"
            :src="pipelineAddAgentIcon"
            alt=""
            aria-hidden="true"
          />
          {{ t('agents.newAgent') }}
        </a-menu-item>
        <a-menu-item key="copy">
          <img
            class="add-menu-icon"
            :src="pipelineSelectAgentIcon"
            alt=""
            aria-hidden="true"
          />
          {{ t('agents.selectExistingAgent') }}
        </a-menu-item>
      </a-menu>
    </template>
  </a-dropdown>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { PlusOutlined } from '@ant-design/icons-vue'
import pipelineAddAgentIcon from '@/assets/icons/pipeline-add-agent.svg'
import pipelineSelectAgentIcon from '@/assets/icons/pipeline-select-agent.svg'
import { useAppI18n } from '@/i18n'

defineProps<{ label?: string }>()

const emit = defineEmits<{
  create: []
  copy: []
}>()

const { t } = useAppI18n()
const open = ref(false)

function handleClick({ key }: { key: string | number }) {
  open.value = false
  if (key === 'create') emit('create')
  if (key === 'copy') emit('copy')
}

function close() {
  open.value = false
}

defineExpose({ close })
</script>

<style scoped>
.add-menu-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
  vertical-align: -3px;
}

.add-menu-icon--new {
  box-sizing: border-box;
  padding: 2.35px;
}
</style>
