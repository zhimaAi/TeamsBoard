import type { TaskProgress } from '@/types/pipeline'
import { normalizeClipboardText } from '@/utils/clipboard'

const TASK_IMAGE_MARKER_RE = /\[\[TASK_IMAGE_[^\]]+\]\]/g
// 历史消息在引入 display_content 前会持久化执行用的绝对路径。仅识别
// 我们生成的内部附件路径，避免复制/展示时泄露实现细节。
const TASK_IMAGE_ATTACHMENT_PATH_RE =
  /^\s*(?:[-*]\s*)?.*[/\\]\.goteams-attachments[/\\][0-9a-f-]+\.(?:png|jpe?g|webp|gif)\s*$/gim

export function isUserMessage(item: TaskProgress) {
  return item.record_type === 'user_question' || Boolean(item.user_prompt || item.question)
}

export function initials(name?: string) {
  return (name || 'A').trim().slice(0, 1).toUpperCase()
}

export function resultText(item: TaskProgress) {
  return item.final_result || item.result || (item.status === 'running' || item.status === 'created' ? '正在执行，请稍候…' : '本次执行未返回结果')
}

export function sanitizeUserMessageText(value: string) {
  return normalizeClipboardText(
    value
      .replace(TASK_IMAGE_MARKER_RE, '')
      .replace(TASK_IMAGE_ATTACHMENT_PATH_RE, '已粘贴图片'),
  )
}

export function userMessageText(item: TaskProgress) {
  return sanitizeUserMessageText(item.user_prompt || item.question || '')
}
