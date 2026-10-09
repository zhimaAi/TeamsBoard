<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { KnowledgeReferenceFragment } from '@/types/knowledge-reference'
import { useAppI18n } from '@/i18n'

// S-UI-13: 编辑器选中内容 → 对话输入区的「添加给 Agent」浮层。
// 承载形态说明：该浮层需要一个会抢走焦点的指令输入框；若把它挂在编辑器浮动工具栏内部，
// 输入框获得焦点时编辑器选区失效、浮动工具栏随之卸载，浮层会被连带销毁，
// 因此使用项目标准 Modal（DESIGN.md「Standard Ant Modal」）承载。
const { t } = useAppI18n()

const props = defineProps<{
  open: boolean
  sourceName: string
  fragment: KnowledgeReferenceFragment | null
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  confirm: [instruction: string]
}>()

const PREVIEW_LENGTH = 120

const instruction = ref('')

// 每次打开都清空指令，避免上一次的输入被误用
watch(
  () => props.open,
  (open) => {
    if (open) instruction.value = ''
  },
)

// CF-9：引用长度统一按 Unicode 码点计数
const selectionLength = computed(() => Array.from(props.fragment?.text ?? '').length)

const selectionPreview = computed(() => {
  const text = (props.fragment?.text ?? '').replace(/\s+/g, ' ').trim()
  if (!text) return ''
  return Array.from(text).length > PREVIEW_LENGTH
    ? `${Array.from(text).slice(0, PREVIEW_LENGTH).join('')}…`
    : text
})

const canSend = computed(() => Boolean(props.fragment?.text.trim()))

function close() {
  emit('update:open', false)
}

function confirmSend() {
  if (!canSend.value) return
  emit('confirm', instruction.value.trim())
  emit('update:open', false)
}
</script>

<template>
  <a-modal
    :open="open"
    :title="t('knowledge.agentReferenceTitle')"
    :width="480"
    :ok-text="t('knowledge.agentReferenceSend')"
    :cancel-text="t('knowledge.cancel')"
    :ok-button-props="{ disabled: !canSend }"
    @ok="confirmSend"
    @cancel="close"
  >
    <div class="agent-reference">
      <div class="agent-reference-row">
        <span class="agent-reference-label">{{ t('knowledge.agentReferenceSource') }}</span>
        <span class="agent-reference-value">{{ sourceName || '—' }}</span>
      </div>
      <div class="agent-reference-row">
        <span class="agent-reference-label">{{ t('knowledge.agentReferenceRange') }}</span>
        <span class="agent-reference-value">
          {{ t('knowledge.agentReferenceRangeSummary', { count: selectionLength }) }}
        </span>
      </div>
      <p v-if="selectionPreview" class="agent-reference-preview">{{ selectionPreview }}</p>
      <a-form layout="vertical">
        <a-form-item :label="t('knowledge.agentReferenceInstruction')">
          <a-textarea
            v-model:value="instruction"
            :rows="3"
            :placeholder="t('knowledge.agentReferencePlaceholder')"
          />
        </a-form-item>
      </a-form>
    </div>
  </a-modal>
</template>

<style scoped>
.agent-reference-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 8px;
  font-size: 14px;
}

.agent-reference-label {
  flex: 0 0 auto;
  color: #8c8c8c;
}

.agent-reference-value {
  overflow-wrap: anywhere;
  color: #262626;
}

.agent-reference-preview {
  max-height: 120px;
  overflow-y: auto;
  margin: 0 0 16px;
  padding: 10px 12px;
  border-radius: 8px;
  color: #595959;
  background: #f5f6f8;
  font-size: 13px;
  line-height: 1.6;
}
</style>
