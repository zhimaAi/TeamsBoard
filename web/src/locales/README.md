# 国际化资源规范

- `zh-CN` 是源语言、默认语言和回退语言；其他语言必须与其 namespace、key 和命名占位符完全一致。
- locale 目录下只保留一层 JSON 文件，文件名即 namespace；子模块使用 `domain.module.json` 命名。
- key 使用 `业务域.模块.语义`，不使用中文原文、序号、页面位置或文件路径作为 key。
- 跨两个以上业务域且语义完全一致的文案才进入 `common.json`，领域文案保留在对应 namespace。
- 使用 `{name}`、`{count}` 等命名插值和 Vue I18n 复数语法，不拼接句子，不在资源中保存 HTML。
- 用户输入、后端原样数据、Markdown 正文、命令、日志、文件路径、TeamsBoard、Agent、CLI、API、Git、SSH、Docker Compose、MySQL 和 PgSQL 不翻译。
- 新增模块时同时创建所有 locale 的同名文件并在 `src/i18n/messages.ts` 注册；新增语言时使用 BCP 47 code，并补齐注册表、Ant Design Vue 和 Day.js 映射。
- 英文译文需人工审阅；翻译平台或编辑器插件只能修改 `src/locales/**/*.json`，不得生成运行时代码或提交密钥。

## `<script setup>` 使用注意事项

- 不要在 `defineProps()` 或 `withDefaults()` 的默认值中调用 `t()`。`defineProps()` 的参数会被提升到模块作用域，而 `useI18n()` 返回的 `t` 属于组件 `setup` 作用域，提升后无法访问。
- 需要使用国际化文案作为 Props 默认值时，应先定义 Props，再通过 `computed` 提供回退值。这样也能在切换语言后自动更新文案。

## Vue 模板插值注意事项

- 不要在 `{{ ... }}` 表达式的字符串参数中直接写 `{{name}}` 等包含 `}}` 的示例文本。Vue 模板解析器可能把字符串里的 `}}` 提前识别为当前插值的结束符，导致 `Unterminated string constant`。
- 需要把双花括号语法作为 i18n 插值参数展示时，先在 `<script setup>` 中定义常量，例如 `const variableSyntaxExample = '{{name}}'`，再在模板中使用 `{{ t('key', { syntax: variableSyntaxExample }) }}`。
