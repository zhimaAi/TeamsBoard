'use strict'

const path = require('node:path')
const { app, shell } = require('electron')

// 自定义协议与 internal/cloud/browserlogin.go 的 BrowserLoginScheme /
// BrowserLoginCallbackURL 保持一致；electron-builder.yml 的 protocols 段也要同步。
const PROTOCOL_SCHEME = 'teamsboard'
const CALLBACK_HOST = 'auth'
const CALLBACK_PATH = '/callback'
const MAX_QUERY_VALUE_LENGTH = 512
const CALLBACK_TIMEOUT_MS = 15000

function parseBrowserLoginCallback(rawURL) {
  if (typeof rawURL !== 'string') return null
  const value = rawURL.trim()
  if (!value.startsWith(`${PROTOCOL_SCHEME}://`)) return null

  let parsed
  try {
    parsed = new URL(value)
  } catch {
    return null
  }
  if (parsed.protocol !== `${PROTOCOL_SCHEME}:`) return null
  if (parsed.hostname !== CALLBACK_HOST) return null
  if (parsed.pathname.replace(/\/+$/, '') !== CALLBACK_PATH) return null

  const state = (parsed.searchParams.get('state') || '').trim()
  const ticket = (parsed.searchParams.get('ticket') || '').trim()
  if (!state || !ticket) return null
  if (state.length > MAX_QUERY_VALUE_LENGTH || ticket.length > MAX_QUERY_VALUE_LENGTH) return null
  return { state, ticket }
}

// 从命令行参数里找出浏览器登录回调。Windows/Linux 的协议唤起会把 URL 作为参数传给
// 第二个实例（或首个实例），这里统一按前缀过滤后解析。
function findBrowserLoginCallback(argv) {
  if (!Array.isArray(argv)) return null
  for (const arg of argv) {
    const parsed = parseBrowserLoginCallback(arg)
    if (parsed) return parsed
  }
  return null
}

// 注册 teamsboard:// 协议。开发模式下必须显式给出 execPath 与入口参数，
// 否则系统记住的是 Electron 可执行文件本身，唤起时不会加载本应用。
function registerProtocolClient({ isPackaged, execPath, argv }) {
  if (isPackaged) {
    app.setAsDefaultProtocolClient(PROTOCOL_SCHEME)
    return
  }
  const entry = Array.isArray(argv) ? argv[1] : ''
  if (!entry) {
    app.setAsDefaultProtocolClient(PROTOCOL_SCHEME)
    return
  }
  app.setAsDefaultProtocolClient(PROTOCOL_SCHEME, execPath, [path.resolve(entry)])
}

async function postBrowserLoginCallback(connection, callback, fetchImpl) {
  const url = new URL('/api/local/auth/browser-login/callback', connection.baseURL)
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), CALLBACK_TIMEOUT_MS)
  try {
    const response = await fetchImpl(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-GoTeams-Api-Token': connection.apiToken,
      },
      body: JSON.stringify(callback),
      signal: controller.signal,
    })
    const text = await response.text()
    let payload = null
    try {
      payload = text ? JSON.parse(text) : null
    } catch {
      payload = null
    }
    if (!response.ok) {
      return {
        status: 'failed',
        error: (payload && (payload.error || payload.message)) || `HTTP ${response.status}`,
      }
    }
    return { status: 'ok', user: payload?.user ?? null, adminId: payload?.admin_id ?? '' }
  } catch (error) {
    return { status: 'failed', error: error instanceof Error ? error.message : String(error) }
  } finally {
    clearTimeout(timer)
  }
}

// 浏览器登录深链接处理器。
// - 深链接可能早于 Go sidecar 就绪（协议唤起会新建进程），未就绪时先入队，等 flush 再处理；
// - 结果通过 notifyRenderer 交给渲染层，渲染层据此刷新登录态，无需依赖轮询时序。
function createBrowserLoginDeepLink({ getConnection, notifyRenderer, fetchImpl = fetch, logger = console }) {
  const pending = []
  let handling = false

  async function run(callback) {
    const connection = getConnection()
    if (!connection || handling) {
      pending.push(callback)
      return
    }
    handling = true
    try {
      const result = await postBrowserLoginCallback(connection, callback, fetchImpl)
      if (result.status !== 'ok') {
        logger.warn?.('[deep-link] 浏览器登录回注失败', result.error)
      }
      notifyRenderer(result)
    } finally {
      handling = false
      const next = pending.shift()
      if (next) void run(next)
    }
  }

  return {
    /** 处理来自 open-url / second-instance / 首次启动 argv 的原始 URL 列表。 */
    handleArgv(argv) {
      const callback = findBrowserLoginCallback(argv)
      if (!callback) return false
      void run(callback)
      return true
    },
    /** 处理 macOS 的 open-url 事件载荷。 */
    handleURL(rawURL) {
      const callback = parseBrowserLoginCallback(rawURL)
      if (!callback) return false
      void run(callback)
      return true
    },
    /** sidecar 就绪后调用，补处理入队的回调。 */
    flush() {
      if (!pending.length || handling) return
      const callback = pending.shift()
      void run(callback)
    },
    hasPending() {
      return pending.length > 0
    },
  }
}

// 打开系统浏览器。只允许 http/https，避免把任意协议交给系统处理。
async function openInBrowser(rawURL) {
  let parsed
  try {
    parsed = new URL(String(rawURL))
  } catch {
    throw new Error('Invalid browser url')
  }
  if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') {
    throw new Error('Unsupported browser url protocol')
  }
  await shell.openExternal(parsed.toString())
  return parsed.toString()
}

module.exports = {
  PROTOCOL_SCHEME,
  parseBrowserLoginCallback,
  findBrowserLoginCallback,
  registerProtocolClient,
  createBrowserLoginDeepLink,
  openInBrowser,
}
