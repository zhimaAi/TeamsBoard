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
 * 扫描整段文本中所有闭合的 `#[...]` 块。
 *
 * 与 collectMarkerRanges 的区别：collectMarkerRanges 依赖已登记的 marker 列表，
 * 本函数只按语法扫描文本中的 #[...] 闭合块，不依赖任何登记表，
 * 用于手输 / 粘贴时把"恰好写对"的 #[xxx] 自动登记到引用表，
 * 渲染层无需额外等待用户走候选菜单。
 *
 * 返回的 start = `#` 的下标，end = `]` 的下标（不含）。
 * 未闭合的 `#`（如 `#task` 或 `#[unclosed`）不会出现在结果中。
 */
export function collectDocumentMentionBlocks(value: string): DocumentMentionRange[] {
  const blocks: DocumentMentionRange[] = []
  if (!value) return blocks
  let scan = 0
  while (scan < value.length) {
    if (
      value[scan] === '#' &&
      scan + 1 < value.length &&
      value[scan + 1] === '['
    ) {
      let end = scan + 2
      let depth = 1
      while (end < value.length && depth > 0) {
        if (value[end] === '[') depth++
        else if (value[end] === ']') depth--
        if (depth === 0) break
        end++
      }
      if (depth === 0) {
        blocks.push({ start: scan, end })
        scan = end + 1
        continue
      }
    }
    scan++
  }
  return blocks
}

/**
 * 从光标位置还原 # 引用上下文。
 * 返回 null 表示当前不该出现引用菜单，调用方据此直接关闭候选列表。
 *
 * 关键约束：行内已闭合的 `#[xxx]` 引用块必须作为整体处理，不能让块内的 `#` /
 * `[` / `]` 干扰引用起点的判定。光标停留在某个闭合块内部时，光标之前没有未闭合
 * 的 `#`，应直接返回 null，而不是把块内的 `#` 当成新引用起点、导致 query 形如
 * `[xxx` 被错误地当作筛选词。
 */
export function resolveDocumentMentionContext(
  value: string,
  caret: number,
): DocumentMentionContext | null {
  const safeCaret = Math.max(0, Math.min(caret, value.length))
  const lineStart = value.lastIndexOf('\n', safeCaret - 1) + 1
  const lineEnd = value.indexOf('\n', safeCaret)
  const lineEndClamped = lineEnd === -1 ? value.length : lineEnd
  // 扫描整行而非只到光标，才能识别完整闭合的 #[...] 块
  const fullLine = value.slice(lineStart, lineEndClamped)
  const caretInLine = safeCaret - lineStart

  // 收集行内所有闭合的 #[...] 块；start = # 的位置，end = ] 的位置（不含）
  const blocks: Array<{ start: number; end: number }> = []
  let scan = 0
  while (scan < fullLine.length) {
    if (
      fullLine[scan] === '#' &&
      scan + 1 < fullLine.length &&
      fullLine[scan + 1] === '['
    ) {
      let end = scan + 2
      let depth = 1
      while (end < fullLine.length && depth > 0) {
        if (fullLine[end] === '[') depth++
        else if (fullLine[end] === ']') depth--
        if (depth === 0) break
        end++
      }
      if (depth === 0) {
        blocks.push({ start: scan, end })
        scan = end + 1
        continue
      }
    }
    scan++
  }

  // 光标停在某个闭合的 #[...] 块内部（含落在 ] 之后、同一行紧邻块尾）：不是新的输入引用
  for (const block of blocks) {
    if (block.start < caretInLine && caretInLine <= block.end) {
      return null
    }
  }

  // 从右往左找到行内最右的 #，并跳过作为 #[...] 块起点的 #（它们只是语法前缀）
  for (let i = caretInLine - 1; i >= 0; i--) {
    if (fullLine[i] !== '#') continue
    const isBlockStart = blocks.some((block) => block.start === i)
    if (isBlockStart) continue
    const query = fullLine.slice(i + 1, caretInLine)
    // 超长段落说明用户只是在正常书写
    if (query.length > MAX_QUERY_LENGTH) return null
    return { start: lineStart + i, end: safeCaret, query }
  }
  return null
}

/**
 * 判断 selection 范围是否与某个已登记 # 引用块重叠。
 * 返回 marker 与 (start, end) 区间，供 ChatComposer 把「点 textarea 某个位置 /
 * 整段选中 marker」翻译成「打开片段预览」。
 *
 * 接受的是 selection 范围 [selectionStart, selectionEnd]，不是单点 caret——
 * ChatComposer 在 click 时会调 selectMarkerAtCaret 把整段 marker 选中，
 * selectionStart === marker.start 时如果还按单点 caret 判断（左开区间）就
 * 永远不命中，preview 也就打不开。这里用「区间有交集」覆盖这两种情形：
 * - 单点 caret 落在 marker 内（selectionStart === selectionEnd）
 * - 整段选中 marker（selectionStart === marker.start, selectionEnd === marker.end）
 *
 * 与 resolveDocumentMentionContext 的语义差别：
 * - resolveDocumentMentionContext: 关心光标前是否有「未闭合」的 #（picker 用）。
 *   光标落在闭合块内时返回 null，让 picker 自动收起。
 * - resolveDocumentMentionPreview:  关心 selection 范围是否覆盖某个已登记的 #[...] 块
 *   （preview 用）。picker 那种返回 null 的位置在这里恰恰是要命中。
 *
 * 复用 collectMarkerRanges：它已经处理同名 marker 多次出现、相邻区间的合并，
 * 直接遍历区间即可，不重复实现 marker 区间扫描。
 */
export interface DocumentMentionPreviewContext {
  marker: DocumentMentionMarker
  start: number
  end: number
}

export function resolveDocumentMentionPreview(
  value: string,
  selectionStart: number,
  selectionEnd: number,
  markers: DocumentMentionMarker[],
): DocumentMentionPreviewContext | null {
  if (!value) return null
  const safeStart = Math.max(0, Math.min(selectionStart, value.length))
  const safeEnd = Math.max(safeStart, Math.min(selectionEnd, value.length))
  const ranges = collectMarkerRanges(
    value,
    markers.map((item) => item.marker),
  )
  for (const range of ranges) {
    // 区间交集：[safeStart, safeEnd) 与 [range.start, range.end) 有交集即命中
    if (safeStart < range.end && range.start < safeEnd) {
      const text = value.slice(range.start, range.end)
      const matched = markers.find((item) => item.marker === text)
      if (!matched) continue
      return { marker: matched, start: range.start, end: range.end }
    }
  }
  return null
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
  /** S-UI-22: 引用已失效（知识库文档被删除或磁盘文件不存在），渲染为失效胶囊 */
  invalid?: boolean
  /**
   * 知识库选中片段引用，与整篇文档引用视觉区分。
   * 任务产出文件引用恒为 undefined。
   */
  isFragment?: boolean
  /**
   * 知识库选中片段原文，用于悬浮 tooltip 展示（S-UI-13）。
   * 聊天记录渲染时无此数据。
   */
  fragmentText?: string
}

/** 渲染层需要的最小引用信息：标记原文 + 文件名 */
export interface DocumentMentionMarker {
  marker: string
  name: string
  /**
   * 知识库选中片段引用标记（`fragment != null`）。
   * 渲染层据此与整篇文档引用做视觉区分；任务产出文件引用恒为 false。
   */
  isFragment?: boolean
  /**
   * 知识库选中片段的原文（S-UI-13 / D3-H2）。
   * 用于输入框引用胶囊的悬浮 tooltip 展示；聊天记录无此数据。
   */
  fragmentText?: string
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
  const matched = markers.find((item) => item.marker === marker)
  const name = matched?.name
  return {
    text: marker,
    reference: true,
    syntaxPrefix,
    label,
    syntaxSuffix,
    fileName: name || label.split(' · ')[0],
    isFragment: matched?.isFragment ?? false,
    fragmentText: matched?.fragmentText,
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
