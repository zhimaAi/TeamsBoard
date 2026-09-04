import { computed, ref } from 'vue'
import apiClient from '@/api/client'

const MAX_IMAGE_SIZE = 10 * 1024 * 1024
const MAX_IMAGE_TOTAL_SIZE = 30 * 1024 * 1024
const MAX_IMAGE_COUNT = 8
const SUPPORTED_IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif'])

interface PastedImage {
  file: File
  marker: string
  markdown?: string
}

interface SavedTaskAttachment {
  name: string
  relative_path: string
  absolute_path: string
  markdown: string
}

const TASK_IMAGE_MARKER_PATTERN = /\[\[TASK_IMAGE_[^\]]+\]\]|!?\[[^\n]*\]\(attachments\/pending-[0-9a-f-]{36}(?:\.[a-z0-9]{1,16})?\)/i
const TASK_IMAGE_MARKER_REPLACE_PATTERN = /\[\[TASK_IMAGE_[^\]]+\]\]|!?\[[^\n]*\]\(attachments\/pending-[0-9a-f-]{36}(?:\.[a-z0-9]{1,16})?\)/gi
const BLOCK_HTML_TAGS = new Set([
  'address',
  'article',
  'aside',
  'blockquote',
  'div',
  'figure',
  'footer',
  'header',
  'li',
  'main',
  'ol',
  'p',
  'pre',
  'section',
  'table',
  'ul',
])

function normalizeLineEndings(value: string) {
  return value.replace(/\r\n?/g, '\n')
}

function ensureLineBreak(value: string) {
  return value && !value.endsWith('\n') ? `${value}\n` : value
}

function contentFromClipboardHtml(html: string, markers: string[]) {
  if (!html.trim() || !markers.length || typeof DOMParser === 'undefined') return ''

  const document = new DOMParser().parseFromString(html, 'text/html')
  if (!document.body.querySelector('img')) return ''

  let markerIndex = 0
  let content = ''
  const appendMarker = () => {
    if (!markers[markerIndex]) return false
    content = ensureLineBreak(content)
    content += markers[markerIndex]
    markerIndex += 1
    content = ensureLineBreak(content)
    return true
  }
  const visit = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      content += (node.textContent || '').replace(/\u00a0/g, ' ')
      return
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return

    const element = node as HTMLElement
    const tag = element.tagName.toLowerCase()
    if (tag === 'img') {
      appendMarker()
      return
    }
    if (tag === 'br') {
      content = ensureLineBreak(content)
      return
    }

    const isBlock = BLOCK_HTML_TAGS.has(tag)
    if (isBlock) content = ensureLineBreak(content)
    element.childNodes.forEach(visit)
    if (isBlock) content = ensureLineBreak(content)
  }

  document.body.childNodes.forEach(visit)
  return markerIndex === markers.length ? normalizeLineEndings(content) : ''
}

export function formatPastedImageContent(
  pastedText: string,
  pastedHtml: string,
  markers: string[],
  textBefore: string,
  textAfter: string,
): string {
  const htmlContent = contentFromClipboardHtml(pastedHtml, markers)
  const text = htmlContent || normalizeLineEndings(pastedText)
  const imageContent = htmlContent
    ? text
    : `${text}${text && !text.endsWith('\n') ? '\n' : ''}${markers.join('\n')}`
  const beforeImage =
    textBefore && !textBefore.endsWith('\n') && !imageContent.startsWith('\n') ? '\n' : ''
  const afterImage =
    textAfter && !textAfter.startsWith('\n') && !imageContent.endsWith('\n') ? '\n' : ''
  return `${beforeImage}${imageContent}${afterImage}`
}

export function stripTaskImageMarkers(content: string) {
  return content.replace(TASK_IMAGE_MARKER_REPLACE_PATTERN, '')
}

function validateFile(file: File) {
  if (file.size <= 0) throw new Error('文件内容为空')
  if (file.size > MAX_IMAGE_SIZE) throw new Error('文件大小不能超过 10MB')
}

function validateImage(file: File) {
  validateFile(file)
  if (!SUPPORTED_IMAGE_TYPES.has(file.type)) {
    throw new Error('仅支持 PNG、JPEG、WebP 或 GIF 图片')
  }
}

function attachmentDisplayName(file: File) {
  return file.name.trim() || (SUPPORTED_IMAGE_TYPES.has(file.type) ? 'pasted-image' : 'attachment')
}

function attachmentExtension(file: File) {
  const extension = attachmentDisplayName(file).match(/\.([a-z0-9]{1,16})$/i)?.[0]
  if (extension) return extension.toLowerCase()
  return {
    'image/png': '.png',
    'image/jpeg': '.jpg',
    'image/webp': '.webp',
    'image/gif': '.gif',
  }[file.type] || ''
}

function escapeMarkdownLabel(value: string) {
  return value.replace(/\\/g, '\\\\').replace(/\[/g, '\\[').replace(/\]/g, '\\]')
}

function pendingAttachmentMarkdown(file: File, identifier: string) {
  const name = escapeMarkdownLabel(attachmentDisplayName(file))
  const target = `attachments/pending-${identifier}${attachmentExtension(file)}`
  return SUPPORTED_IMAGE_TYPES.has(file.type) ? `![${name}](${target})` : `[${name}](${target})`
}

function readFileAsDataUrl(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(new Error(`读取文件 ${attachmentDisplayName(file)} 失败`))
    reader.onload = () => resolve(String(reader.result || ''))
    reader.readAsDataURL(file)
  })
}

/**
 * 用临时 Markdown 标记保留附件在输入中的位置，存入任务的 attachments
 * 目录后再原位替换为可移植的相对 Markdown 引用。
 */
export function useTaskImageAttachments() {
  const attachments = ref<PastedImage[]>([])
  const savingCount = ref(0)
  const isSaving = computed(() => savingCount.value > 0)

  function addFiles(files: File[], imageOnly = false) {
    if (!files.length) return []
    files.forEach(imageOnly ? validateImage : validateFile)
    if (attachments.value.length + files.length > MAX_IMAGE_COUNT) {
      throw new Error(`最多可上传 ${MAX_IMAGE_COUNT} 个文件`)
    }
    const totalSize = files.reduce(
      (total, file) => total + file.size,
      attachments.value.reduce((total, attachment) => total + attachment.file.size, 0),
    )
    if (totalSize > MAX_IMAGE_TOTAL_SIZE) {
      throw new Error('上传文件总大小不能超过 30MB')
    }

    const added = files.map((file) => {
      const identifier = crypto.randomUUID()
      return {
        file,
        marker: pendingAttachmentMarkdown(file, identifier),
      }
    })
    attachments.value.push(...added)
    return added
  }

  function addImages(files: File[]) {
    return addFiles(files, true)
  }

  async function uploadAttachmentsToTask(taskUuid: string) {
    if (!taskUuid) throw new Error('缺少任务信息，无法保存附件')
    const pendingAttachments = attachments.value.filter((attachment) => !attachment.markdown)
    if (!pendingAttachments.length) return

    savingCount.value += 1
    try {
      for (const attachment of pendingAttachments) {
        const formData = new FormData()
        formData.append('file', attachment.file, attachment.file.name || 'attachment')
        const saved = await apiClient.post<SavedTaskAttachment>(
          `/tasks/${encodeURIComponent(taskUuid)}/files/attachments`,
          formData,
        )
        attachment.markdown = saved.markdown
      }
    } finally {
      savingCount.value -= 1
    }
  }

  async function buildDraftAttachmentInputs(content = '') {
    const pendingAttachments = attachments.value.filter(
      (attachment) => !content || content.includes(attachment.marker),
    )
    if (!pendingAttachments.length) return []
    savingCount.value += 1
    try {
      return await Promise.all(
        pendingAttachments.map(async (attachment) => ({
          name: attachment.file.name || 'attachment',
          data: await readFileAsDataUrl(attachment.file),
          marker: attachment.marker,
        })),
      )
    } finally {
      savingCount.value -= 1
    }
  }

  function clear() {
    attachments.value = []
  }

  function resolveAttachmentMarkdown(content: string, caretPosition = content.length) {
    let serialized = content
    let caret = Math.min(Math.max(caretPosition, 0), content.length)
    for (const attachment of attachments.value) {
      if (!attachment.markdown) continue
      const markerIndex = serialized.indexOf(attachment.marker)
      if (markerIndex < 0) continue
      serialized = serialized.replace(attachment.marker, attachment.markdown)
      if (markerIndex < caret) {
        caret += attachment.markdown.length - attachment.marker.length
      }
    }
    if (TASK_IMAGE_MARKER_PATTERN.test(serialized)) {
      throw new Error('附件尚未保存，请重新选择后发送')
    }
    return { content: serialized, caret }
  }

  function removeAttachmentMarkers(content: string) {
    let value = content
    for (const attachment of attachments.value) {
      value = value.split(attachment.marker).join('')
    }
    return stripTaskImageMarkers(value)
  }

  return {
    isSaving,
    addFiles,
    addImages,
    buildDraftAttachmentInputs,
    uploadAttachmentsToTask,
    resolveAttachmentMarkdown,
    removeAttachmentMarkers,
    clear,
  }
}
