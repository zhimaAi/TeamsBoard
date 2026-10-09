import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { KnowledgeReferenceDraft } from '@/types/knowledge-reference'

/**
 * S-UI-13: 知识库页与任务对话页是两个独立路由（`web/src/router/index.ts`），需要一个跨页投递通道，
 * 用于把编辑器选中内容送到对话输入区。
 *
 * 该 store 只承载这一条待投递引用队列，不做本地持久化（`web/AGENTS.md`：Store 默认不做本地持久化）。
 */
export const useKnowledgeReferenceStore = defineStore('knowledge-reference', () => {
  const pending = ref<KnowledgeReferenceDraft[]>([])

  function enqueue(draft: KnowledgeReferenceDraft) {
    pending.value = [...pending.value, draft]
  }

  /** 取走全部待投递引用并清空队列（消费端为 ChatComposer，由 T5 实现） */
  function consume(): KnowledgeReferenceDraft[] {
    const drafts = [...pending.value]
    pending.value = []
    return drafts
  }

  function clear() {
    pending.value = []
  }

  return { pending, enqueue, consume, clear }
})
