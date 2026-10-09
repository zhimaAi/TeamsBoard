<template>
  <div class="markdown-preview-shell">
    <a-empty v-if="!content.trim()" :description="emptyText || t('components.markdown.empty')" />
    <article
      v-else
      ref="previewRef"
      class="markdown-preview"
      @click="handleAttachmentClick"
      v-html="renderedContent"
    ></article>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import MarkdownIt from 'markdown-it'
import apiClient from '@/api/client'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

const props = withDefaults(defineProps<{
  content?: string
  emptyText?: string
  taskUuid?: string
}>(), {
  content: '',
  emptyText: '',
  taskUuid: '',
})
const emit = defineEmits<{
  'attachments-loaded': []
}>()

const markdown = new MarkdownIt({
  breaks: true,
  linkify: true,
})

const previewRef = ref<HTMLElement>()
const objectUrls = new Set<string>()
let hydrationVersion = 0

function taskAttachmentPath(value: string) {
  const normalized = value.trim().replace(/\\/g, '/').replace(/[?#].*$/, '')
  const match = normalized.match(/(?:^|\/)attachments\/([0-9a-f-]{36}\.[a-z0-9]{1,16})$/i)
  return match ? `attachments/${match[1]}` : ''
}

const originalImageRenderer = markdown.renderer.rules.image
markdown.renderer.rules.image = (tokens, index, options, env, self) => {
  const token = tokens[index]
  const path = env.taskUuid ? taskAttachmentPath(token.attrGet('src') || '') : ''
  if (path) {
    const sourceIndex = token.attrIndex('src')
    if (sourceIndex >= 0) token.attrs?.splice(sourceIndex, 1)
    token.attrSet('data-task-attachment-path', path)
  }
  return originalImageRenderer
    ? originalImageRenderer(tokens, index, options, env, self)
    : self.renderToken(tokens, index, options)
}

const originalLinkOpenRenderer = markdown.renderer.rules.link_open
markdown.renderer.rules.link_open = (tokens, index, options, env, self) => {
  const token = tokens[index]
  const path = env.taskUuid ? taskAttachmentPath(token.attrGet('href') || '') : ''
  if (path) {
    token.attrSet('href', '#')
    token.attrSet('data-task-attachment-path', path)
  }
  return originalLinkOpenRenderer
    ? originalLinkOpenRenderer(tokens, index, options, env, self)
    : self.renderToken(tokens, index, options)
}

// `#[文件名]` 是输入框文档引用的持久化标记（见 task-progress/documentMention.ts）。
// 在这里把它解析成行内节点，让历史消息也渲染成文件标签，而不是把内部语法直接摆给用户看。
const DOC_REFERENCE_RE = /^#\[([^\]\n]{1,200})\]/

// S-UI-13: 知识库选中片段会在文件名前追加「· 选」后缀，聊天记录里沿用相同标记。
// 历史消息没有运行时引用登记，按 i18n 文本识别「· 选」并区分渲染。
function splitDocReferenceLabel(label: string) {
  const fragmentBadge = ' · '
  const badgeIdx = label.lastIndexOf(fragmentBadge)
  if (badgeIdx < 0) return { fileName: label, badge: '', hasBadge: false }
  // 段尾的「 · 选」= 选中片段标记；其余「 · N」= 重复去重序号，归文件名部分
  const possible = label.slice(badgeIdx + fragmentBadge.length)
  // 去重序号是纯数字，选中片段 badge 长度很短且不是纯数字
  const isFragmentBadge = !/^\d+$/.test(possible)
  return {
    fileName: label.slice(0, badgeIdx),
    badge: isFragmentBadge ? `${fragmentBadge}${possible}` : '',
    hasBadge: isFragmentBadge,
  }
}

markdown.inline.ruler.before('emphasis', 'doc_reference', (state, silent) => {
  if (state.src.charCodeAt(state.pos) !== 0x23 /* # */) return false
  const match = DOC_REFERENCE_RE.exec(state.src.slice(state.pos))
  if (!match) return false
  if (!silent) {
    const token = state.push('doc_reference', '', 0)
    // 先存整段标签：渲染时再按当前语言识别「· 选」后缀，避免 locale 切换后渲染与解析错位
    token.content = match[1]
  }
  state.pos += match[0].length
  return true
})

markdown.renderer.rules.doc_reference = (tokens, index) => {
  const { fileName, badge, hasBadge } = splitDocReferenceLabel(tokens[index].content)
  // badge 是从 marker 原始文本里切出的 i18n 后缀（如「 · 选」），直接拼入 HTML 不需要再次转义
  const className = hasBadge
    ? 'markdown-doc-reference markdown-doc-reference-fragment'
    : 'markdown-doc-reference'
  const titleAttr = hasBadge
    ? ` title="${markdown.utils.escapeHtml(t('workflows.task.progress.referenceFragmentHint'))}"`
    : ''
  const badgeHtml = hasBadge
    ? `<span class="markdown-doc-reference-badge">${markdown.utils.escapeHtml(badge)}</span>`
    : ''
  return `<span class="${className}"${titleAttr}>${markdown.utils.escapeHtml(fileName)}${badgeHtml}</span>`
}

// 渲染前归一化：统一换行符、折叠连续空行、去行尾空格，避免脏换行产生多余间距
function normalizeMarkdown(text: string) {
  return text
    .replace(/\r\n/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .replace(/[ \t]+\n/g, '\n')
    .trim()
}

const renderedContent = computed(() =>
  markdown.render(normalizeMarkdown(props.content || ''), { taskUuid: props.taskUuid }),
)

watch(
  () => [renderedContent.value, props.taskUuid] as const,
  () => void hydrateAttachmentImages(),
  { immediate: true },
)

async function hydrateAttachmentImages() {
  const version = ++hydrationVersion
  releaseObjectUrls()
  await nextTick()
  if (!props.taskUuid || version !== hydrationVersion) return
  const images = [...(previewRef.value?.querySelectorAll<HTMLImageElement>('img[data-task-attachment-path]') || [])]
  await Promise.all(
    images.map(async (image) => {
      const path = image.dataset.taskAttachmentPath
      if (!path) return
      try {
        const blob = await apiClient.getBlob(
          `/tasks/${encodeURIComponent(props.taskUuid)}/files/raw`,
          { path },
        )
        if (version !== hydrationVersion || !image.isConnected) return
        const objectUrl = URL.createObjectURL(blob)
        objectUrls.add(objectUrl)
        image.src = objectUrl
      } catch {
        if (version !== hydrationVersion || !image.isConnected) return
        image.classList.add('attachment-load-failed')
        image.title = t('components.markdown.attachmentLoadFailed')
      }
    }),
  )
  if (version === hydrationVersion) emit('attachments-loaded')
}

async function handleAttachmentClick(event: MouseEvent) {
  const target = event.target
  if (!(target instanceof Element)) return
  const anchor = target.closest<HTMLAnchorElement>('a[data-task-attachment-path]')
  const path = anchor?.dataset.taskAttachmentPath
  if (!anchor || !path || !props.taskUuid) return
  event.preventDefault()
  try {
    const blob = await apiClient.getBlob(
      `/tasks/${encodeURIComponent(props.taskUuid)}/files/raw`,
      { path },
    )
    const objectUrl = URL.createObjectURL(blob)
    const download = document.createElement('a')
    download.href = objectUrl
    download.download = anchor.textContent?.trim() || path.split('/').at(-1) || 'attachment'
    download.click()
    URL.revokeObjectURL(objectUrl)
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('components.markdown.attachmentDownloadFailed'))
  }
}

function releaseObjectUrls() {
  for (const objectUrl of objectUrls) URL.revokeObjectURL(objectUrl)
  objectUrls.clear()
}

onBeforeUnmount(() => {
  hydrationVersion += 1
  releaseObjectUrls()
})
</script>

<style scoped>
.markdown-preview-shell {
  min-height: 100%;
  padding: 20px 26px 48px;
  box-sizing: border-box;
  background: #fff;
}

.markdown-preview-shell :deep(.ant-empty) {
  margin-top: 96px;
}

/*
 * 文档引用标签。与输入框内的引用标签、输入框上方的引用 chip 保持同一套视觉：
 * 100px 圆角胶囊、浅灰底、深色字，只展示文件名，不暴露 #[...] 内部语法。
 */
.markdown-preview :deep(.markdown-doc-reference) {
  display: inline-block;
  max-width: 240px;
  padding: 1px 8px;
  border-radius: 100px;
  color: rgba(0, 0, 0, 0.9);
  background: #f2f2f2;
  font-size: 13px;
  font-weight: 500;
  line-height: 20px;
  vertical-align: -4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/*
 * S-UI-13: 知识库选中片段引用，与整篇文档引用做视觉区分。
 * 与 ChatComposer 输入框的 .mention-token-fragment 保持同一套配色（Primary Soft + 主蓝文字），
 * 避免用户在输入框 / 聊天记录 / 历史消息里看到三种不同的「选中片段」表现。
 */
.markdown-preview :deep(.markdown-doc-reference-fragment) {
  color: #3157e2;
  background: #e5efff;
}

/*
 * S-UI-13: 「· 选」后缀比主文件名淡一些，视觉上不抢主名风头。
 * 整体胶囊是蓝底，所以 badge 用更浅的蓝或半透明白。
 */
.markdown-preview :deep(.markdown-doc-reference-fragment .markdown-doc-reference-badge) {
  color: rgba(49, 87, 226, 0.6);
}

.markdown-preview {
  /* 覆盖消息卡片继承的 pre-wrap：渲染 HTML 里的格式换行不再产生间距，
     内容中的真实换行仍由 breaks: true 转成 <br> 保留 */
  white-space: normal;
  overflow-wrap: anywhere;
  color: #2f3440;
  font-size: 14px;
  line-height: 1.6;
}

.markdown-preview :deep(h1),
.markdown-preview :deep(h2),
.markdown-preview :deep(h3),
.markdown-preview :deep(h4),
.markdown-preview :deep(h5),
.markdown-preview :deep(h6) {
  color: #1f2430;
  font-weight: 650;
  line-height: 1.35;
  margin: 1.2em 0 0.5em;
}

.markdown-preview :deep(h1) {
  padding-bottom: 0.35em;
  border-bottom: 1px solid #e7eaf0;
  font-size: 1.9em;
}

.markdown-preview :deep(h2) {
  padding-bottom: 0.3em;
  border-bottom: 1px solid #eef0f4;
  font-size: 1.5em;
}

.markdown-preview :deep(h3) {
  font-size: 1.24em;
}

.markdown-preview :deep(h4) {
  font-size: 1em;
}

.markdown-preview :deep(h5) {
  font-size: 0.92em;
}

.markdown-preview :deep(h6) {
  font-size: 0.85em;
}

.markdown-preview :deep(p),
.markdown-preview :deep(ul),
.markdown-preview :deep(ol),
.markdown-preview :deep(blockquote),
.markdown-preview :deep(pre),
.markdown-preview :deep(table) {
  margin: 0.8em 0;
}

.markdown-preview :deep(> :first-child) {
  margin-top: 0;
}

.markdown-preview :deep(> :last-child) {
  margin-bottom: 0;
}

.markdown-preview :deep(a) {
  color: #3a7a3a;
  text-decoration: none;
}

.markdown-preview :deep(a:hover) {
  text-decoration: underline;
}

.markdown-preview :deep(code) {
  padding: 0.15em 0.35em;
  border-radius: 4px;
  background: #f3f6f1;
  color: #b04b67;
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 0.88em;
}

.markdown-preview :deep(pre) {
  overflow-x: auto;
  padding: 14px 16px;
  border-radius: 7px;
  background: #1f2430;
  font-size: 0.92em;
  line-height: 1.6;
}

.markdown-preview :deep(pre code) {
  padding: 0;
  background: transparent;
  color: #e8eaf0;
}

.markdown-preview :deep(blockquote) {
  margin-left: 0;
  padding-left: 14px;
  border-left: 3px solid #9fb39a;
  color: #686f7c;
}

.markdown-preview :deep(table) {
  width: 100%;
  border-collapse: collapse;
}

.markdown-preview :deep(th),
.markdown-preview :deep(td) {
  padding: 7px 10px;
  border: 1px solid #dfe3ea;
  text-align: left;
}

.markdown-preview :deep(th) {
  background: #f7f9f4;
}

.markdown-preview :deep(img) {
  max-width: 100%;
}

.markdown-preview :deep(img.attachment-load-failed) {
  min-width: 120px;
  min-height: 64px;
  border: 1px dashed #d9d9d9;
}

@media (max-width: 640px) {
  .markdown-preview-shell {
    padding: 16px 18px 36px;
  }
}
</style>
