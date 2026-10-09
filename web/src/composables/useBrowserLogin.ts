import { computed, ref } from 'vue'
import apiClient, { ApiError } from '@/api/client'
import { refreshSession } from '@/composables/useCloudSession'
import { t } from '@/i18n'
import { ipcErrorMessage } from '@/utils/ipcError'

/** 浏览器登录：客户端申请会话 → 系统浏览器完成登录 → 深链接回注登录态。 */
interface BrowserLoginStartResponse {
  state: string
  browser_url: string
  expires_in: number
}

type BrowserLoginBackendStatus = 'pending' | 'succeeded' | 'failed' | 'expired' | 'cancelled'

interface BrowserLoginStatusResponse {
  status: BrowserLoginBackendStatus
  user?: { id: string; username: string; display_name?: string; avatar?: string; role?: string }
  admin_id?: string
  error?: string
}

// 轮询间隔：浏览器里的登录/注册需要用户操作，1.5 秒足够及时又不打扰后端。
const POLL_INTERVAL_MS = 1500
const POLL_TIMEOUT_MS = 10 * 60 * 1000

// 会话探测比状态轮询低频：每 N 次状态轮询顺带查一次本地会话，避免刷屏式打点。
const SESSION_PROBE_EVERY = 2

// 侧边栏账号入口与团队工作登录引导卡片共用同一次登录流程，所以状态放在模块级单例，
// 而不是每个组件各起一份轮询。
const pending = ref(false)
const errorMessage = ref('')
const state = ref('')
let pollTimer: ReturnType<typeof setInterval> | null = null
let deadline = 0
let pollTicks = 0
let desktopUnsubscribe: (() => void) | null = null
let settleResolvers: Array<(ok: boolean) => void> = []

function stopPolling() {
  if (pollTimer !== null) {
    clearInterval(pollTimer)
    pollTimer = null
  }
  desktopUnsubscribe?.()
  desktopUnsubscribe = null
}

function settle(ok: boolean, message = '') {
  stopPolling()
  state.value = ''
  pending.value = false
  errorMessage.value = ok ? '' : message
  const resolvers = settleResolvers
  settleResolvers = []
  for (const resolve of resolvers) resolve(ok)
}

function waitForSettle(): Promise<boolean> {
  return new Promise((resolve) => {
    settleResolvers.push(resolve)
  })
}

// 以本地会话为最终依据收尾：浏览器里完成的登录可能对应「前端先取消过的那次会话」，
// 深链接兑换成功后推送未必落在当前这次等待上，此时状态轮询会一直停在 pending。
// 会话接口一旦显示已登录，就说明登录确实成功了，直接收尾。
async function settleIfSessionLoggedIn(): Promise<boolean> {
  try {
    const session = await refreshSession()
    if (!session.cloud_logged_in) return false
    settle(true)
    return true
  } catch {
    return false
  }
}

async function pollOnce() {
  if (!state.value) return
  if (Date.now() > deadline) {
    settle(false, t('teamwork.login.expired'))
    return
  }
  pollTicks += 1
  if (pollTicks % SESSION_PROBE_EVERY === 0 && (await settleIfSessionLoggedIn())) return
  try {
    const result = await apiClient.get<BrowserLoginStatusResponse>('/auth/browser-login/status', {
      state: state.value,
    })
    if (result.status === 'pending') return
    if (result.status === 'succeeded') {
      await refreshSession()
      settle(true)
      return
    }
    if (result.status === 'failed') {
      settle(false, result.error || t('teamwork.login.failed'))
      return
    }
    settle(false, t('teamwork.login.expired'))
  } catch (error) {
    // 会话已被清理（后端重启、过期回收）时不再空转到超时，直接提示重新发起。
    if (error instanceof ApiError && error.status === 404) {
      settle(false, t('teamwork.login.expired'))
      return
    }
    // 网络抖动或后端重启时保持轮询，超时由 deadline 兜底。
  }
}

function startPolling() {
  stopPolling()
  deadline = Date.now() + POLL_TIMEOUT_MS
  pollTicks = 0
  pollTimer = setInterval(() => {
    void pollOnce()
  }, POLL_INTERVAL_MS)
  void pollOnce()

  // Electron 主进程收到 teamsboard:// 深链接并完成回注后会主动推送，界面无需等下一次轮询。
  // 推送只是提示：登录态以会话接口为准，避免重复投递或时序竞争导致误报成功。
  desktopUnsubscribe = window.goteamsDesktop?.onBrowserLoginResult?.((result) => {
    if (result.status !== 'ok') {
      if (!pending.value) return
      settle(false, result.error || t('teamwork.login.failed'))
      return
    }
    void settleIfSessionLoggedIn()
  }) ?? null
}

/**
 * 发起浏览器登录。返回是否登录成功，便于调用方在成功后继续下一步。
 * 重复调用时复用进行中的流程。
 */
async function startBrowserLogin(): Promise<boolean> {
  if (pending.value) return waitForSettle()

  errorMessage.value = ''
  pending.value = true
  try {
    const desktop = window.goteamsDesktop
    if (desktop?.startBrowserLogin) {
      const result = await desktop.startBrowserLogin()
      state.value = result.state
    } else {
      // 浏览器直连 dev（无 Electron 桥）：自己申请会话并新开标签页。
      const result = await apiClient.post<BrowserLoginStartResponse>('/auth/browser-login/start', {})
      state.value = result.state
      window.open(result.browser_url, '_blank', 'noopener')
    }
  } catch (error) {
    settle(false, ipcErrorMessage(error, t('teamwork.login.failed')))
    return false
  }

  startPolling()
  return waitForSettle()
}

/** 取消进行中的浏览器登录（关闭弹窗、切换页面等）。 */
async function cancelBrowserLogin(): Promise<void> {
  const current = state.value
  if (!current) {
    if (pending.value) settle(false)
    return
  }
  try {
    const result = await apiClient.post<{ status?: BrowserLoginBackendStatus }>('/auth/browser-login/cancel', { state: current })
    if (result.status === 'succeeded') {
      await refreshSession()
      settle(true)
      return
    }
    if (result.status === 'pending') {
      // 深链接已开始兑换，继续等回注结果。
      return
    }
  } catch {
    // 取消失败不影响本地收尾，会话会在后端超时后自动过期。
  }
  settle(false)
}

export function useBrowserLogin() {
  return {
    pending: computed(() => pending.value),
    errorMessage: computed(() => errorMessage.value),
    startBrowserLogin,
    cancelBrowserLogin,
  }
}
