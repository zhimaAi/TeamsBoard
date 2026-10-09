<template>
  <!-- 团队需求描述是网页端富文本编辑器产出的 HTML，直接当 Markdown 渲染会把标签源码
       暴露给用户；这里按内容形态分流：HTML 原样渲染，其余走 Markdown 预览。
       两个类名与 goteams 网页端 RichTextEditor 只读态完全一致：
       <div class="rich-text-content rendered-rich-text-content" v-html="readOnlyHtml" /> -->
  <div
    v-if="html"
    class="rich-text-content rendered-rich-text-content"
    v-html="html"
  />
  <MarkdownPreview
    v-else
    :content="content"
    :empty-text="emptyText"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import MarkdownPreview from '@/components/MarkdownPreview.vue'

const props = withDefaults(defineProps<{
  content?: string
  emptyText?: string
}>(), {
  content: '',
  emptyText: '',
})

function isHTML(value: string) {
  return /<[a-z][\s\S]*>/i.test(value)
}

const html = computed(() => {
  const value = (props.content || '').trim()
  return isHTML(value) ? value : ''
})
</script>

<style scoped>
/*
 * 只读富文本排版，逐条取自 goteams 网页端
 * web/src/components/common/RichTextEditor.vue 里 .rich-text-content（基础）
 * 与 .rendered-rich-text-content（只读覆盖）合并后的最终生效值，
 * 保证同一份需求在网页端与客户端看起来一致。
 *
 * 与网页端唯一的差异：网页端容器自带 padding: 16px 与 min-height，这里由外层
 * 容器（配置弹窗的 .work-item-description）控制留白与高度，故不重复设置。
 */
.rich-text-content {
  color: #333;
  font-size: 14px;
  line-height: 1.7;
  overflow-wrap: anywhere;
}

.rich-text-content :deep(p) {
  margin: 0 0 8px;
}

.rich-text-content :deep(p:last-child) {
  margin-bottom: 0;
}

.rich-text-content :deep(ol),
.rich-text-content :deep(ul) {
  margin: 8px 0;
  padding-left: 24px;
}

.rich-text-content :deep(h1),
.rich-text-content :deep(h2),
.rich-text-content :deep(h3),
.rich-text-content :deep(h4),
.rich-text-content :deep(h5),
.rich-text-content :deep(h6) {
  margin: 16px 0 8px;
  color: #262626;
  font-weight: 600;
  line-height: 1.35;
}

.rich-text-content :deep(h1) {
  font-size: 24px;
}

.rich-text-content :deep(h2) {
  font-size: 20px;
}

.rich-text-content :deep(h3) {
  font-size: 17px;
}

.rich-text-content :deep(h4) {
  font-size: 16px;
}

.rich-text-content :deep(h5) {
  font-size: 14px;
}

.rich-text-content :deep(h6) {
  font-size: 13px;
}

/* 网页端的表格由 tiptap 多包一层 .tableWrapper 承担横向滚动。 */
.rich-text-content :deep(.tableWrapper) {
  margin: 12px 0;
  overflow-x: auto;
}

.rich-text-content :deep(table) {
  width: 100%;
  margin: 12px 0;
  overflow: hidden;
  border-collapse: collapse;
  table-layout: fixed;
}

.rich-text-content :deep(th),
.rich-text-content :deep(td) {
  position: relative;
  box-sizing: border-box;
  min-width: 60px;
  padding: 8px 10px;
  border: 1px solid #d9d9d9;
  text-align: left;
  vertical-align: top;
}

.rich-text-content :deep(th) {
  background: #fafafa;
  font-weight: 600;
}

.rich-text-content :deep(th > p:last-of-type),
.rich-text-content :deep(td > p:last-of-type) {
  margin-bottom: 0;
}

.rich-text-content :deep(a) {
  display: inline-block;
  max-width: 100%;
  color: #1677ff;
  text-decoration: underline;
  overflow-wrap: anywhere;
}

.rich-text-content :deep(img) {
  display: inline-block;
  max-width: 100%;
  height: auto;
  margin: 4px 0;
  vertical-align: bottom;
}

.rich-text-content :deep(blockquote) {
  margin: 12px 0;
  padding: 4px 12px;
  border-left: 3px solid #d9d9d9;
  color: #666;
  background: #fafafa;
}

.rich-text-content :deep(pre) {
  margin: 12px 0;
  padding: 12px;
  overflow: auto;
  border-radius: 4px;
  background: #f5f5f5;
  font-family: Consolas, "Courier New", monospace;
  white-space: pre;
}

.rich-text-content :deep(code) {
  padding: 2px 4px;
  border-radius: 3px;
  background: #f5f5f5;
  font-family: Consolas, "Courier New", monospace;
  font-size: 0.92em;
}

.rich-text-content :deep(pre code) {
  padding: 0;
  background: transparent;
}
</style>
