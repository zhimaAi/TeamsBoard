import { t } from '@/i18n'

export async function copyText(text: string): Promise<void> {
  const value = normalizeClipboardText(text)
  try {
    copyTextWithExecCommand(value)
    return
  } catch {
    // execCommand 可能因焦点失败，再尝试 Clipboard API
  }
  await navigator.clipboard.writeText(value)
}

export function normalizeClipboardText(text: string): string {
  return (text ?? '').replace(/\r\n?/g, '\n').replace(/[\u200e\u200f\u202a-\u202e]/g, '')
}

function copyTextWithExecCommand(text: string) {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.top = '0'
  textarea.style.left = '0'
  textarea.style.opacity = '0'
  textarea.style.pointerEvents = 'none'
  document.body.appendChild(textarea)

  const selection = document.getSelection()
  const ranges = selection
    ? Array.from({ length: selection.rangeCount }, (_, index) => selection.getRangeAt(index))
    : []

  textarea.focus()
  textarea.select()
  textarea.setSelectionRange(0, textarea.value.length)

  try {
    if (!document.execCommand('copy')) throw new Error(t('components.feedback.copyFailed'))
  } finally {
    textarea.remove()
    if (selection) {
      selection.removeAllRanges()
      for (const range of ranges) selection.addRange(range)
    }
  }
}
