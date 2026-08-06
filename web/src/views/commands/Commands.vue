<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import apiClient from '@/api/client'

type CommandKind = 'git' | 'docker'
type Project = {
  id: number
  name: string
  ssh_profile_id?: number
  ssh_name?: string
  remote_work_dir?: string
  compose_file_path?: string
}
type RunResult = {
  run_id: number
  status: string
  exit_code: number
  output: string
  duration_ms: number
}
type HistoryRun = {
  id: number
  command_type: CommandKind
  project_id: number
  command: string
  args: string[]
  status: string
  exit_code: number
  output: string
  duration_ms: number
  started_at: number
}
type Suggestion = {
  token: string
  label: string
  description: string
  icon?: string
}
type ResultMessage = {
  command: string
  status: 'running' | 'success' | 'failed'
  output: string
  durationMs?: number
  exitCode?: number
}

const gitActions = [
  ['status', '查看工作区状态'],
  ['log', '查看提交历史'],
  ['branch', '查看或管理分支'],
  ['diff', '查看代码差异'],
  ['fetch', '拉取远程引用'],
  ['pull', '拉取并合并'],
  ['push', '推送当前分支'],
  ['add', '添加文件到暂存区'],
  ['commit', '提交暂存区内容'],
  ['checkout', '切换分支或文件'],
  ['stash', '管理暂存修改'],
  ['merge', '合并分支'],
  ['rebase', '变基分支'],
] as const

const dockerActions = [
  ['ps', '查看容器状态'],
  ['up', '创建并启动服务'],
  ['down', '停止并移除服务'],
  ['logs', '查看服务日志'],
  ['build', '构建服务镜像'],
  ['restart', '重启服务'],
  ['stop', '停止服务'],
  ['start', '启动已有服务'],
  ['config', '查看解析后的 Compose 配置'],
  ['images', '查看服务镜像'],
] as const

const topCommands: Suggestion[] = [
  { token: 'git', label: '/git', description: 'Git 项目常用操作', icon: '⑂' },
  { token: 'docker', label: '/docker', description: 'Docker Compose 服务操作', icon: '◇' },
  { token: 'history', label: '/history', description: '查看最近命令执行记录', icon: '◷' },
  { token: 'help', label: '/help', description: '显示命令使用说明', icon: '?' },
]

const inputRef = ref<HTMLInputElement>()
const resultListRef = ref<HTMLElement>()
const inputText = ref('')
const activeIndex = ref(0)
const showSuggestions = ref(false)
const executing = ref(false)
const gitProjects = ref<Project[]>([])
const dockerProjects = ref<Project[]>([])
const history = ref<HistoryRun[]>([])
const results = ref<ResultMessage[]>([])

// Results are ordered ascending by execution sequence; new results are appended to the end and auto-scrolled to the latest.
function appendResult(item: ResultMessage) {
  results.value.push(item)
  nextTick(() => {
    if (resultListRef.value) {
      resultListRef.value.scrollTop = resultListRef.value.scrollHeight
    }
  })
}

function parseTokens(value: string): string[] {
  const normalized = value.trim().replace(/^\/+/, '')
  if (!normalized) return []
  const matches = normalized.match(/(?:[^\s"]+|"[^"]*")+/g) || []
  return matches.map((item) => item.startsWith('"') && item.endsWith('"') ? item.slice(1, -1) : item)
}

function quoteToken(value: string): string {
  return /\s/.test(value) ? `"${value.replace(/"/g, '\\"')}"` : value
}

function actionsFor(kind: string) {
  return kind === 'git' ? gitActions : kind === 'docker' ? dockerActions : []
}

function projectsFor(kind: string) {
  return kind === 'git' ? gitProjects.value : kind === 'docker' ? dockerProjects.value : []
}

function tokenMatches(value: string, query: string) {
  return value.toLowerCase().includes(query.toLowerCase())
}

const suggestions = computed<Suggestion[]>(() => {
  const tokens = parseTokens(inputText.value)
  const trailingSpace = /\s$/.test(inputText.value)
  if (tokens.length === 0) return topCommands

  const top = tokens[0].toLowerCase()
  if (!['git', 'docker', 'history', 'help'].includes(top)) {
    return topCommands.filter((item) =>
      tokenMatches(item.token, top) || tokenMatches(item.description, top))
  }
  if (top === 'help') return []
  if (top === 'history') {
    const query = tokens.length > 1 && !trailingSpace ? tokens[1] : ''
    return [
      { token: 'git', label: 'git', description: '只看 Git 历史', icon: '⑂' },
      { token: 'docker', label: 'docker', description: '只看 Docker 历史', icon: '◇' },
    ].filter((item) => !query || tokenMatches(item.token, query))
  }

  const actions = actionsFor(top)
  if (tokens.length === 1 || (tokens.length === 2 && !trailingSpace && !actions.some(([name]) => name === tokens[1]))) {
    const query = tokens.length === 2 ? tokens[1] : ''
    return actions
      .filter(([name, description]) => !query || tokenMatches(name, query) || tokenMatches(description, query))
      .map(([name, description]) => ({ token: name, label: name, description, icon: '›' }))
  }

  const action = tokens[1]
  if (!actions.some(([name]) => name === action)) return []
  const projects = projectsFor(top)
  if (tokens.length <= 2 || (tokens.length === 3 && !trailingSpace)) {
    const query = tokens.length === 3 ? tokens[2] : ''
    return projects
      .filter((project) => !query || tokenMatches(project.name, query) || String(project.id) === query)
      .map((project) => ({
        token: quoteToken(project.name),
        label: project.name,
        description: top === 'git'
          ? `${project.ssh_name || 'SSH'} · ${project.remote_work_dir || '远程目录'}`
          : project.compose_file_path || 'Compose 项目',
        icon: top === 'git' ? '⑂' : '◇',
      }))
  }
  return []
})

const commandReady = computed(() => {
  const tokens = parseTokens(inputText.value)
  if (tokens[0] === 'help') return true
  if (tokens[0] === 'history') return tokens.length <= 2
  if (!['git', 'docker'].includes(tokens[0])) return false
  const actionExists = actionsFor(tokens[0]).some(([name]) => name === tokens[1])
  const projectExists = projectsFor(tokens[0]).some((project) =>
    project.name.toLowerCase() === (tokens[2] || '').toLowerCase() || String(project.id) === tokens[2])
  return actionExists && projectExists
})

const breadcrumb = computed(() => {
  const tokens = parseTokens(inputText.value)
  if (!tokens.length) return '选择一级命令'
  if (tokens[0] === 'history') return 'history › 可选类型'
  if (tokens[0] === 'help') return 'help'
  if (tokens.length === 1) return `${tokens[0]} › 选择操作`
  if (tokens.length === 2) return `${tokens[0]} › ${tokens[1]} › 选择项目`
  return `${tokens[0]} › ${tokens[1]} › ${tokens[2]} › 可继续输入参数`
})

const nextHint = computed(() => {
  const tokens = parseTokens(inputText.value)
  if (!tokens.length) return '输入 / 唤起命令，↑↓ 选择，Tab 或 Enter 补全'
  if (suggestions.value.length) return `下一步：${breadcrumb.value}`
  if (commandReady.value) return '命令已完整，按 Enter 执行；项目名后的内容会作为命令参数'
  return '未识别的命令；输入 /help 查看帮助'
})

function replaceCurrentToken(token: string) {
  const raw = inputText.value
  const tokens = parseTokens(raw)
  const trailingSpace = /\s$/.test(raw)
  let stage = 0
  if (tokens.length === 0) stage = 0
  else if (['git', 'docker'].includes(tokens[0])) {
    if (tokens.length === 1) stage = 1
    else if (tokens.length === 2 && !trailingSpace &&
      !actionsFor(tokens[0]).some(([name]) => name === tokens[1])) stage = 1
    else if (tokens.length <= 2) stage = 2
    else stage = 2
  } else if (tokens[0] === 'history') {
    stage = 1
  }

  const prefix = tokens.slice(0, stage)
  const next = [...prefix, token]
  inputText.value = `/${next.join(' ')} `
  activeIndex.value = 0
  showSuggestions.value = true
  nextTick(() => inputRef.value?.focus())
}

function selectSuggestion(item: Suggestion) {
  replaceCurrentToken(item.token)
}

function quickSelect(command: Suggestion) {
  inputText.value = `/${command.token} `
  showSuggestions.value = true
  activeIndex.value = 0
  nextTick(() => inputRef.value?.focus())
}

function handleInput() {
  showSuggestions.value = inputText.value.trimStart().startsWith('/')
  activeIndex.value = 0
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown' && suggestions.value.length) {
    event.preventDefault()
    activeIndex.value = (activeIndex.value + 1) % suggestions.value.length
    return
  }
  if (event.key === 'ArrowUp' && suggestions.value.length) {
    event.preventDefault()
    activeIndex.value = (activeIndex.value - 1 + suggestions.value.length) % suggestions.value.length
    return
  }
  if (event.key === 'Tab' && suggestions.value.length) {
    event.preventDefault()
    selectSuggestion(suggestions.value[activeIndex.value] || suggestions.value[0])
    return
  }
  if (event.key === 'Escape') {
    showSuggestions.value = false
    return
  }
  if (event.key === 'Enter') {
    event.preventDefault()
    if (!commandReady.value && suggestions.value.length) {
      selectSuggestion(suggestions.value[activeIndex.value] || suggestions.value[0])
    } else {
      void executeCommand()
    }
  }
}

async function loadData() {
  const [git, docker, recent] = await Promise.allSettled([
    apiClient.get<{ data: Project[] }>('/commands/git/projects'),
    apiClient.get<{ data: Project[] }>('/commands/docker/projects'),
    apiClient.get<{ data: HistoryRun[] }>('/commands/history', { page_size: 12 }),
  ])
  if (git.status === 'fulfilled') gitProjects.value = git.value.data || []
  if (docker.status === 'fulfilled') dockerProjects.value = docker.value.data || []
  if (recent.status === 'fulfilled') history.value = recent.value.data || []
}

async function showHistory(type?: string) {
  const response = await apiClient.get<{ data: HistoryRun[] }>('/commands/history', {
    type: type || undefined,
    page_size: 50,
  })
  history.value = response.data || []
  appendResult({
    command: `/history${type ? ` ${type}` : ''}`,
    status: 'success',
    output: history.value.length
      ? history.value.map((item) =>
          `${new Date(item.started_at).toLocaleString()}  ${item.command_type} ${item.command} ${(item.args || []).join(' ')}  [${item.status}]`,
        ).join('\n')
      : '暂无命令历史',
  })
}

async function executeCommand() {
  if (executing.value) return
  const rawCommand = inputText.value.trim()
  const tokens = parseTokens(rawCommand)
  if (!tokens.length) return
  if (tokens[0] === 'help') {
    appendResult({
      command: rawCommand,
      status: 'success',
      output: [
        '/git <操作> <项目> [参数...]',
        '/docker <操作> <项目> [参数...]',
        '/history [git|docker]',
        '',
        '输入一级命令后使用 ↑↓ 选择，Tab/Enter 补全；命令完整后按 Enter 执行。',
      ].join('\n'),
    })
    inputText.value = ''
    showSuggestions.value = false
    return
  }
  if (tokens[0] === 'history') {
    await showHistory(tokens[1])
    inputText.value = ''
    showSuggestions.value = false
    return
  }
  if (!commandReady.value) {
    message.warning('命令还不完整')
    return
  }

  const kind = tokens[0] as CommandKind
  const project = projectsFor(kind).find((item) =>
    item.name.toLowerCase() === tokens[2].toLowerCase() || String(item.id) === tokens[2])
  if (!project) {
    message.error('未找到项目配置')
    return
  }
  const resultItem: ResultMessage = {
    command: rawCommand,
    status: 'running',
    output: '正在执行...',
  }
  appendResult(resultItem)
  executing.value = true
  showSuggestions.value = false
  try {
    const response = await apiClient.post<RunResult>(`/commands/${kind}/run`, {
      project_id: project.id,
      command: tokens[1],
      args: tokens.slice(3),
    })
    resultItem.status = response.status === 'success' ? 'success' : 'failed'
    resultItem.output = response.output || '（无输出）'
    resultItem.durationMs = response.duration_ms
    resultItem.exitCode = response.exit_code
    await loadData()
  } catch (error) {
    resultItem.status = 'failed'
    resultItem.output = error instanceof Error ? error.message : '执行失败'
  } finally {
    executing.value = false
    inputText.value = ''
    nextTick(() => inputRef.value?.focus())
  }
}

onMounted(async () => {
  await loadData()
  inputRef.value?.focus()
})
</script>

<template>
  <div class="command-page">
    <div class="command-main">
      <section v-if="results.length === 0" class="welcome">
        <div class="eyebrow">COMMAND PALETTE</div>
        <h1>命令快捷操作</h1>
        <p>像 dtool 一样，从一个斜杠命令开始，逐级选择操作和目标。</p>
        <div class="top-command-grid">
          <button
            v-for="command in topCommands"
            :key="command.token"
            type="button"
            class="top-command"
            @click="quickSelect(command)"
          >
            <span class="top-command-icon">{{ command.icon }}</span>
            <span class="top-command-copy">
              <strong>{{ command.label }}</strong>
              <small>{{ command.description }}</small>
            </span>
          </button>
        </div>
      </section>

      <section v-else ref="resultListRef" class="result-list">
        <article v-for="(item, index) in results" :key="`${item.command}-${index}`" class="result-card">
          <header>
            <code>{{ item.command }}</code>
            <a-tag :color="item.status === 'success' ? 'success' : item.status === 'running' ? 'processing' : 'error'">
              {{ item.status }}
            </a-tag>
            <span v-if="item.durationMs !== undefined">{{ item.durationMs }} ms</span>
            <span v-if="item.exitCode !== undefined">exit {{ item.exitCode }}</span>
          </header>
          <pre>{{ item.output }}</pre>
        </article>
      </section>
    </div>

    <div class="command-dock">
      <div v-show="showSuggestions && suggestions.length" class="suggestions">
        <div class="suggestion-breadcrumb">{{ breadcrumb }}</div>
        <button
          v-for="(item, index) in suggestions"
          :key="`${item.token}-${index}`"
          type="button"
          :class="['suggestion-item', { active: index === activeIndex }]"
          @mouseenter="activeIndex = index"
          @mousedown.prevent="selectSuggestion(item)"
        >
          <span class="suggestion-icon">{{ item.icon }}</span>
          <strong>{{ item.label }}</strong>
          <span>{{ item.description }}</span>
          <kbd>↵</kbd>
        </button>
      </div>
      <div class="input-shell">
        <span class="prompt-mark">›</span>
        <input
          ref="inputRef"
          v-model="inputText"
          type="text"
          placeholder="输入 /git、/docker 或 /history"
          autocomplete="off"
          spellcheck="false"
          @input="handleInput"
          @focus="showSuggestions = inputText.trimStart().startsWith('/') || !inputText"
          @keydown="handleKeydown"
        />
        <button type="button" :disabled="executing || !commandReady" @click="executeCommand">
          {{ executing ? '执行中' : '执行' }}
        </button>
      </div>
      <div class="next-hint">{{ nextHint }}</div>
    </div>
  </div>
</template>

<style scoped>
.command-page {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 104px);
  max-width: 980px;
  width: 100%;
  margin: 0 auto;
  position: relative;
  color: #172033;
}
.command-main { display: flex; flex-direction: column; flex: 1 1 auto; min-height: 0; overflow: hidden; width: 100%; }
.welcome { padding-top: 7vh; text-align: center; }
.eyebrow { color: #3157e2; font-size: 12px; font-weight: 700; letter-spacing: .16em; }
.welcome h1 { margin: 10px 0 6px; font-size: 30px; }
.welcome p { margin: 0 0 28px; color: #7a8498; }
.top-command-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.top-command {
  display: flex; align-items: center; gap: 14px; padding: 17px 18px; text-align: left;
  background: #fff; border: 1px solid #e4e8f0; border-radius: 10px; cursor: pointer;
  transition: border-color .15s, box-shadow .15s, transform .15s;
}
.top-command:hover { border-color: #99abef; box-shadow: 0 8px 24px rgb(49 87 226 / 10%); transform: translateY(-1px); }
.top-command-icon { display: grid; place-items: center; width: 36px; height: 36px; color: #3157e2; background: #eef2ff; border-radius: 8px; font-size: 19px; }
.top-command-copy { display: flex; flex-direction: column; gap: 3px; }
.top-command-copy strong { font-family: Consolas, monospace; font-size: 15px; }
.top-command-copy small { color: #8a93a5; }
.result-list { display: flex; flex-direction: column; gap: 14px; flex: 1 1 auto; min-height: 0; overflow-y: auto; padding: 4px 8px 4px 2px; }
.result-list::-webkit-scrollbar { width: 8px; }
.result-list::-webkit-scrollbar-thumb { background: #c9ced6; border-radius: 4px; }
.result-list::-webkit-scrollbar-thumb:hover { background: #aab2bd; }
.result-card { flex-shrink: 0; overflow: hidden; background: #fff; border: 1px solid #e4e8f0; border-radius: 10px; }
.result-card header { display: flex; align-items: center; gap: 10px; padding: 11px 14px; color: #7a8498; border-bottom: 1px solid #edf0f5; font-size: 12px; }
.result-card header code { flex: 1; color: #25314a; font-size: 13px; }
.result-card pre { margin: 0; padding: 16px; color: #dbe4f1; background: #111827; white-space: pre-wrap; word-break: break-word; }
.command-dock { flex-shrink: 0; width: 100%; margin-top: 12px; }
.suggestions { max-height: 320px; overflow: hidden auto; margin-bottom: 8px; background: rgb(255 255 255 / 98%); border: 1px solid #dfe4ee; border-radius: 10px; box-shadow: 0 14px 40px rgb(23 32 51 / 16%); }
.suggestion-breadcrumb { padding: 9px 14px; color: #8a93a5; background: #f8f9fc; border-bottom: 1px solid #edf0f5; font-size: 12px; }
.suggestion-item { display: grid; grid-template-columns: 24px 150px minmax(0, 1fr) 28px; align-items: center; width: 100%; padding: 10px 14px; color: #26324a; text-align: left; background: transparent; border: 0; cursor: pointer; }
.suggestion-item.active { background: #eef2ff; }
.suggestion-item > span:nth-child(3) { overflow: hidden; color: #8490a5; text-overflow: ellipsis; white-space: nowrap; }
.suggestion-icon { color: #3157e2; }
.suggestion-item kbd { color: #98a1b2; background: #fff; border: 1px solid #dfe4ee; border-radius: 4px; text-align: center; }
.input-shell { display: flex; align-items: center; gap: 10px; padding: 8px 9px 8px 15px; background: #fff; border: 1px solid #cfd6e3; border-radius: 12px; box-shadow: 0 12px 36px rgb(23 32 51 / 14%); }
.input-shell:focus-within { border-color: #3157e2; box-shadow: 0 12px 36px rgb(49 87 226 / 16%); }
.prompt-mark { color: #3157e2; font-size: 24px; font-weight: 600; }
.input-shell input { flex: 1; min-width: 0; padding: 7px 0; font: 15px/1.5 Consolas, Monaco, monospace; border: 0; outline: 0; }
.input-shell button { padding: 9px 18px; color: #fff; background: #3157e2; border: 0; border-radius: 8px; cursor: pointer; }
.input-shell button:disabled { color: #a6aebb; background: #edf0f5; cursor: default; }
.next-hint { padding: 7px 14px 0; color: #8a93a5; font-size: 12px; }
@media (max-width: 900px) {
  .top-command-grid { grid-template-columns: 1fr; }
}
</style>
