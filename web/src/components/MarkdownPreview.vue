<script setup lang="ts">
import { computed } from 'vue'
import MarkdownIt from 'markdown-it'

const props = withDefaults(defineProps<{
  content?: string
  emptyText?: string
}>(), {
  content: '',
  emptyText: '暂无 Markdown 内容',
})

const markdown = new MarkdownIt({
  breaks: true,
  linkify: true,
})

const renderedContent = computed(() => markdown.render(props.content || ''))
</script>

<template>
  <div class="markdown-preview-shell">
    <a-empty v-if="!content.trim()" :description="emptyText" />
    <article v-else class="markdown-preview" v-html="renderedContent"></article>
  </div>
</template>

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

.markdown-preview {
  overflow-wrap: anywhere;
  color: #2f3440;
  font-size: 14px;
  line-height: 1.75;
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

.markdown-preview :deep(p),
.markdown-preview :deep(ul),
.markdown-preview :deep(ol),
.markdown-preview :deep(blockquote),
.markdown-preview :deep(pre),
.markdown-preview :deep(table) {
  margin: 0.8em 0;
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

@media (max-width: 640px) {
  .markdown-preview-shell {
    padding: 16px 18px 36px;
  }
}
</style>
