import type { TaskProgress } from '@/types/pipeline'

export function isUserMessage(item: TaskProgress) {
  return item.record_type === 'user_question' || Boolean(item.user_prompt || item.question)
}

export function initials(name?: string) {
  return (name || 'A').trim().slice(0, 1).toUpperCase()
}

export function resultText(item: TaskProgress) {
  return item.final_result || item.result || (item.status === 'running' || item.status === 'created' ? '正在执行，请稍候…' : '本次执行未返回结果')
}
