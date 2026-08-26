/**
 * 启动鉴权 token 的获取与缓存。
 * 桌面模式经 window.goteamsDesktop.getApiToken()（Electron 主进程生成）获取；
 * 纯浏览器 dev（无 Electron）时由后端 /auth/session 响应的 api_token 兜底。
 * token 只保存在内存中，不写 localStorage、不进日志。
 */

const SESSION_PATH = '/api/local/auth/session'

let cachedToken = ''
let inflight: Promise<string> | null = null

async function fetchTokenFromSession(): Promise<string> {
  try {
    const response = await fetch(SESSION_PATH, {
      credentials: 'same-origin',
      cache: 'no-store',
    })
    if (!response.ok) return ''
    const data = (await response.json()) as { api_token?: unknown }
    return typeof data.api_token === 'string' ? data.api_token : ''
  } catch {
    // 后端尚未就绪时返回空串，由调用方（轮询/重连）自行重试
    return ''
  }
}

/** 获取 token：缓存命中直接返回；否则按 bridge → session 顺序获取（单飞）。 */
export function getApiToken(): Promise<string> {
  if (cachedToken) return Promise.resolve(cachedToken)
  if (!inflight) {
    inflight = resolveToken().finally(() => {
      inflight = null
    })
  }
  return inflight
}

async function resolveToken(): Promise<string> {
  const bridge = window.goteamsDesktop?.getApiToken
  if (bridge) {
    try {
      const token = await bridge()
      if (typeof token === 'string' && token) {
        cachedToken = token
        return token
      }
    } catch {
      // bridge 不可用（如浏览器直连 dev），走 session 兜底
    }
  }
  const token = await fetchTokenFromSession()
  if (token) cachedToken = token
  return token
}

/** main.ts 完成 /auth/session 握手后注入兜底 token。 */
export function seedApiToken(token: string) {
  if (token) cachedToken = token
}

/** token 失效（403 / WS 断开）时清除缓存，下次请求重新获取。 */
export function invalidateApiToken() {
  cachedToken = ''
}
