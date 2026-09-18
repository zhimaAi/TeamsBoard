import { readFile, readdir } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const projectRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const localesRoot = path.join(projectRoot, 'src', 'locales')
const sourceLocale = 'zh-CN'
const messagesFile = path.join(projectRoot, 'src', 'i18n', 'messages.ts')
const configFile = path.join(projectRoot, 'src', 'i18n', 'config.ts')
const htmlPattern = /<\/?[a-z][^>]*>/i
const placeholderPattern = /\{([A-Za-z][A-Za-z0-9_]*)\}/g

function fail(message) {
  throw new Error(`[i18n] ${message}`)
}

async function readJson(filepath) {
  try {
    return JSON.parse(await readFile(filepath, 'utf8'))
  } catch (error) {
    fail(`无法解析 ${path.relative(projectRoot, filepath)}：${error.message}`)
  }
}

function flattenMessages(value, prefix = '', result = new Map()) {
  if (typeof value === 'string') {
    if (!value.trim()) fail(`${prefix} 的译文不能为空`)
    if (htmlPattern.test(value)) fail(`${prefix} 不允许包含 HTML`)
    const placeholders = [...new Set(
      [...value.matchAll(placeholderPattern)].map((match) => match[1]),
    )].sort()
    result.set(prefix, placeholders)
    return result
  }

  if (!value || Array.isArray(value) || typeof value !== 'object') {
    fail(`${prefix || '<root>'} 的叶子必须是字符串`)
  }

  for (const [key, child] of Object.entries(value)) {
    flattenMessages(child, prefix ? `${prefix}.${key}` : key, result)
  }
  return result
}

async function loadLocale(locale) {
  const directory = path.join(localesRoot, locale)
  const files = (await readdir(directory, { withFileTypes: true }))
    .filter((entry) => entry.isFile() && entry.name.endsWith('.json'))
    .map((entry) => entry.name)
    .sort()

  const namespaces = new Map()
  for (const filename of files) {
    const namespace = filename.slice(0, -'.json'.length)
    namespaces.set(namespace, flattenMessages(await readJson(path.join(directory, filename))))
  }
  return namespaces
}

function assertSameItems(actual, expected, label) {
  const missing = expected.filter((item) => !actual.includes(item))
  const extra = actual.filter((item) => !expected.includes(item))
  if (missing.length || extra.length) {
    fail(`${label} 不一致；缺少：${missing.join(', ') || '无'}；多出：${extra.join(', ') || '无'}`)
  }
}

const localeEntries = (await readdir(localesRoot, { withFileTypes: true }))
  .filter((entry) => entry.isDirectory())
  .map((entry) => entry.name)
  .sort()

if (!localeEntries.includes(sourceLocale)) fail(`缺少源语言目录 ${sourceLocale}`)

const sourceNamespaces = await loadLocale(sourceLocale)
const sourceNamespaceNames = [...sourceNamespaces.keys()]

for (const locale of localeEntries) {
  const namespaces = await loadLocale(locale)
  assertSameItems([...namespaces.keys()], sourceNamespaceNames, `${locale} namespace`)

  for (const namespace of sourceNamespaceNames) {
    const sourceMessages = sourceNamespaces.get(namespace)
    const targetMessages = namespaces.get(namespace)
    const sourceKeys = [...sourceMessages.keys()]
    const targetKeys = [...targetMessages.keys()]
    assertSameItems(targetKeys, sourceKeys, `${locale}/${namespace} key`)

    for (const key of sourceKeys) {
      assertSameItems(
        targetMessages.get(key),
        sourceMessages.get(key),
        `${locale}/${namespace}.${key} 占位符`,
      )
    }
  }
}

const messagesSource = await readFile(messagesFile, 'utf8')
const registeredFiles = [...messagesSource.matchAll(/@\/locales\/([^/]+\/[^'"\n]+\.json)/g)]
  .map((match) => match[1].replaceAll('\\', '/'))
  .sort()
const resourceFiles = localeEntries
  .flatMap((locale) => sourceNamespaceNames.map((namespace) => `${locale}/${namespace}.json`))
  .sort()

assertSameItems(registeredFiles, resourceFiles, 'messages.ts 运行时注册资源')

const configSource = await readFile(configFile, 'utf8')
const registeredLocales = [...configSource.matchAll(/\bcode:\s*['"]([^'"]+)['"]/g)]
  .map((match) => match[1])
  .sort()
assertSameItems(registeredLocales, localeEntries, 'config.ts 语言注册表')

// JSON 合法且双语对齐，不代表 vue-i18n 能编译：`@` 是链接消息语法、`|` 是复数分隔符。
// 这里用运行时编译器逐条校验，避免此类文案直到页面渲染才抛错。
const { createI18n } = await import('vue-i18n')

async function buildRuntimeMessages(locale) {
  const tree = {}
  for (const namespace of sourceNamespaceNames) {
    const namespaceMessages = await readJson(path.join(localesRoot, locale, `${namespace}.json`))
    const segments = namespace.split('.')
    let node = tree
    for (const segment of segments.slice(0, -1)) node = node[segment] ??= {}
    node[segments.at(-1)] = namespaceMessages
  }
  return tree
}

function collectKeys(value, prefix = '', result = []) {
  if (typeof value === 'string') {
    result.push(prefix)
    return result
  }
  for (const [key, child] of Object.entries(value)) {
    collectKeys(child, prefix ? `${prefix}.${key}` : key, result)
  }
  return result
}

let compiledMessages = 0
for (const locale of localeEntries) {
  const localeMessages = await buildRuntimeMessages(locale)
  const { global: translator } = createI18n({
    legacy: false,
    locale,
    fallbackLocale: false,
    messages: { [locale]: localeMessages },
    missingWarn: false,
    fallbackWarn: false,
  })
  for (const key of collectKeys(localeMessages)) {
    try {
      translator.t(key)
    } catch (error) {
      fail(`${locale} ${key} 无法被 vue-i18n 编译：${error.message}`)
    }
    compiledMessages++
  }
}

console.log(
  `[i18n] ${localeEntries.length} 个语言、${sourceNamespaceNames.length} 个 namespace、`
  + `${compiledMessages} 条文案编译检查通过`,
)
