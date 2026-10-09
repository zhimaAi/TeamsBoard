import type { Pinia } from 'pinia'
import apiClient from '@/api/client'
import { seedApiToken } from '@/api/token'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'

export interface SessionResponse {
  local_authenticated: boolean
  cloud_logged_in: boolean
  user?: {
    id: string
    username: string
    display_name?: string
    avatar?: string
    role?: string
  }
  api_token?: string
}

// token 不进入日志（浏览器直连 dev 时 session 响应会携带 api_token）
export function redactSession(session: SessionResponse): string {
  const rest: Record<string, unknown> = { ...session }
  delete rest.api_token
  return JSON.stringify(rest)
}

// dev 模式后端（go run）启动比 Vite 慢，窗口打开时后端可能还在编译；
// 轮询直到后端就绪并返回真实会话，避免把「后端未就绪」误判为「未登录」而停在登录页。
export async function fetchSession(retryForMs = 0): Promise<SessionResponse> {
  const deadline = Date.now() + retryForMs
  let attempts = 0
  for (;;) {
    attempts++
    try {
      const session = await apiClient.get<SessionResponse>('/auth/session')
      console.log('[session] 探测成功 response=' + redactSession(session))
      return session
    } catch (error) {
      if (Date.now() >= deadline) {
        console.warn('[session] 探测超时', { attempts, error: String(error) })
        throw error
      }
      await new Promise((resolve) => setTimeout(resolve, 1000))
    }
  }
}

/** 以本地 API 的会话响应为唯一真相，同步前端登录态。 */
export function applySession(session: SessionResponse, pinia?: Pinia): void {
  console.log('[session] 应用会话结果', redactSession(session))
  const authStore = useAuthStore(pinia)
  const appStore = useAppStore(pinia)
  if (session.api_token) seedApiToken(session.api_token)
  authStore.setLocalSession(session.local_authenticated)
  if (session.cloud_logged_in && session.user) {
    authStore.setCloudAuth('http-only-session', {
      id: session.user.id,
      username: session.user.username,
      displayName: session.user.display_name || session.user.username,
      avatar: session.user.avatar,
      role: session.user.role,
    })
    appStore.setCloudStatus('online')
  } else {
    authStore.clearCloudAuth()
    appStore.setCloudStatus('offline')
  }
}

/** 重新拉取并应用会话（浏览器登录回注成功后调用）。 */
export async function refreshSession(pinia?: Pinia): Promise<SessionResponse> {
  const session = await fetchSession()
  applySession(session, pinia)
  return session
}
