import type { MentionableTaskFile } from '@/types/task-files'

/** 光标处的一次 # 引用上下文 */
export interface DocumentMentionContext {
  /** # 在整段文本中的下标 */
  start: number
  /** 光标位置，即引用片段的结束下标 */
  end: number
  /** # 之后到光标之间的查询词 */
  query: string
}

/** 命中的文本区间，用于在列表里高亮 */
export interface DocumentMentionRange {
  start: number
  end: number
}

/** 候选文件：附带匹配质量与文件名高亮区间 */
export interface DocumentMentionCandidate extends MentionableTaskFile {
  score: number
  nameRanges: DocumentMentionRange[]
}

export interface DocumentMentionSegment {
  text: string
  matched: boolean
}

const MAX_QUERY_LENGTH = 120
const MAX_RESULTS = 50
const MAX_RANGES_PER_FILE = 8

/**
 * 从光标位置还原 # 引用上下文。
 * 返回 null 表示当前不该出现引用菜单，调用方据此直接关闭候选列表。
 */
export function resolveDocumentMentionContext(
  value: string,
  caret: number,
): DocumentMentionContext | null {
  const safeCaret = Math.max(0, Math.min(caret, value.length))
  const lineStart = value.lastIndexOf('\n', safeCaret - 1) + 1
  const currentLine = value.slice(lineStart, safeCaret)
  const hashOffset = currentLine.lastIndexOf('#')
  if (hashOffset < 0) return null

  const query = currentLine.slice(hashOffset + 1)
  // 超长段落说明用户只是在正常书写，已插入完成的 #[名称] 标记也不该再次触发
  if (query.length > MAX_QUERY_LENGTH) return null
  if (query.startsWith('[') && query.includes(']')) return null

  return { start: lineStart + hashOffset, end: safeCaret, query }
}

/**
 * 按查询词过滤候选文件并按匹配质量排序。
 * 查询词按空白拆分为多个关键词，全部关键词都命中才保留，便于 "task util" 这类跨片段检索。
 */
export function matchDocumentMentionFiles(
  files: MentionableTaskFile[],
  query: string,
): DocumentMentionCandidate[] {
  const keywords = tokenize(query)
  if (!keywords.length) {
    // 无查询词：按修改时间倒序，最近产出的文件排在最前；无时间戳的退化为文件名
    return files
      .slice()
      .sort(
        (left, right) =>
          (right.modified_at || 0) - (left.modified_at || 0) ||
          left.name.localeCompare(right.name),
      )
      .slice(0, MAX_RESULTS)
      .map((file) => ({ ...file, score: 0, nameRanges: [] }))
  }

  const candidates: DocumentMentionCandidate[] = []
  for (const file of files) {
    const candidate = scoreDocumentMentionFile(file, keywords)
    if (candidate) candidates.push(candidate)
  }

  candidates.sort(
    (left, right) =>
      right.score - left.score ||
      left.name.length - right.name.length ||
      left.name.localeCompare(right.name),
  )
  return candidates.slice(0, MAX_RESULTS)
}

/**
 * 输入框渲染层的一段内容：普通文本，或一个已登记的 # 引用标签块。
 *
 * 引用块拆成三段渲染，而不是整段着色：
 * - syntaxPrefix / syntaxSuffix 是 `#[` 与 `]` 两个语法字符，只占位、不着色。
 *   它们给标签块提供了左右内边距，同时保证渲染层与 textarea 逐字同宽——
 *   任何宽度差都会让光标位置整体偏移。
 * - label 是真正展示给用户看的文件名，不再暴露 `#[...]` 语法。
 */
export interface DocumentMentionToken {
  text: string
  reference: boolean
  syntaxPrefix?: string
  label?: string
  syntaxSuffix?: string
  /** 引用对应的文件名，用于渲染文件图标 */
  fileName?: string
}

/** 渲染层需要的最小引用信息：标记原文 + 文件名 */
export interface DocumentMentionMarker {
  marker: string
  name: string
}

/**
 * 按已登记的引用标记切分文本，供输入框的富文本渲染层使用。
 *
 * 只认 documentReferences 里登记过的 marker：用户手写的 `#[xxx]` 不会被当成引用，
 * 否则会渲染成标签、看起来生效了，实际提交时并不会替换成路径。
 */
export function splitDocumentMentionTokens(
  value: string,
  markers: DocumentMentionMarker[],
): DocumentMentionToken[] {
  if (!value) return []
  const ranges = collectMarkerRanges(
    value,
    markers.map((item) => item.marker),
  )
  if (!ranges.length) return [{ text: value, reference: false }]

  const tokens: DocumentMentionToken[] = []
  let cursor = 0
  for (const range of ranges) {
    if (range.start > cursor) {
      tokens.push({ text: value.slice(cursor, range.start), reference: false })
    }
    tokens.push(describeReferenceToken(value.slice(range.start, range.end), markers))
    cursor = range.end
  }
  if (cursor < value.length) tokens.push({ text: value.slice(cursor), reference: false })
  return tokens
}

/**
 * 把一段 `#[名称]` 标记拆成「语法前缀 + 文件名 + 语法后缀」。
 * 同名重复引用会带去重序号（`#[task.md · 2]`），文件名要取序号之前的原始名称。
 */
function describeReferenceToken(
  marker: string,
  markers: DocumentMentionMarker[],
): DocumentMentionToken {
  const syntaxPrefix = marker.startsWith('#[') ? '#[' : ''
  const syntaxSuffix = marker.endsWith(']') ? ']' : ''
  const label = marker.slice(syntaxPrefix.length, marker.length - syntaxSuffix.length)
  const name = markers.find((item) => item.marker === marker)?.name
  return {
    text: marker,
    reference: true,
    syntaxPrefix,
    label,
    syntaxSuffix,
    fileName: name || label.split(' · ')[0],
  }
}

/**
 * 找出所有已登记标记在文本里的区间。
 * 输入框渲染层和「把内联胶囊当成整体删除」都依赖它，所以导出复用。
 */
export function collectMarkerRanges(value: string, markers: string[]): DocumentMentionRange[] {
  const ranges: DocumentMentionRange[] = []
  for (const marker of new Set(markers)) {
    if (!marker) continue
    let index = value.indexOf(marker)
    while (index !== -1) {
      ranges.push({ start: index, end: index + marker.length })
      index = value.indexOf(marker, index + marker.length)
    }
  }
  return mergeRanges(ranges)
}

export function splitDocumentMentionHighlight(
  text: string,
  ranges: DocumentMentionRange[],
): DocumentMentionSegment[] {
  if (!ranges.length) return [{ text, matched: false }]

  const segments: DocumentMentionSegment[] = []
  let cursor = 0
  for (const range of ranges) {
    if (range.start > cursor) {
      segments.push({ text: text.slice(cursor, range.start), matched: false })
    }
    segments.push({ text: text.slice(range.start, range.end), matched: true })
    cursor = range.end
  }
  if (cursor < text.length) {
    segments.push({ text: text.slice(cursor), matched: false })
  }
  return segments
}

function tokenize(query: string): string[] {
  return query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
}

function scoreDocumentMentionFile(
  file: MentionableTaskFile,
  keywords: string[],
): DocumentMentionCandidate | null {
  const name = file.name.toLocaleLowerCase()
  const ownerName = (file.owner_step_name || '').toLocaleLowerCase()
  const path = file.path.toLocaleLowerCase()

  const nameRanges: DocumentMentionRange[] = []
  let score = 0

  for (const keyword of keywords) {
    const ranges = findRanges(name, keyword)
    if (ranges.length) {
      // 命中文件名权重最高，位置越靠前越优先
      score += 240 - Math.min(ranges[0].start, 60)
      nameRanges.push(...ranges)
      continue
    }
    if (ownerName.includes(keyword)) {
      score += 80
      continue
    }
    if (path.includes(keyword)) {
      score += 40
      continue
    }
    // 任一关键词未命中即整体淘汰，保证候选列表里都是全部命中的文件
    return null
  }

  return { ...file, score, nameRanges: mergeRanges(nameRanges) }
}

function findRanges(text: string, keyword: string): DocumentMentionRange[] {
  const ranges: DocumentMentionRange[] = []
  let cursor = text.indexOf(keyword)
  while (cursor !== -1 && ranges.length < MAX_RANGES_PER_FILE) {
    ranges.push({ start: cursor, end: cursor + keyword.length })
    cursor = text.indexOf(keyword, cursor + keyword.length)
  }
  return ranges
}

function mergeRanges(ranges: DocumentMentionRange[]): DocumentMentionRange[] {
  if (ranges.length < 2) return ranges
  const sorted = [...ranges].sort((left, right) => left.start - right.start)
  const merged: DocumentMentionRange[] = [{ ...sorted[0] }]
  for (const range of sorted.slice(1)) {
    const last = merged[merged.length - 1]
    if (range.start <= last.end) {
      last.end = Math.max(last.end, range.end)
      continue
    }
    merged.push({ ...range })
  }
  return merged
}
