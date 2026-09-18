import { createApp } from 'vue'
import { createPinia } from 'pinia'
import Antd, { message } from 'ant-design-vue'
import App from './App.vue'
import router from './router'
import apiClient, { setApiFeedbackHandler } from '@/api/client'
import { seedApiToken } from '@/api/token'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { i18n, initializeLocale, startLocaleSync } from '@/i18n'
import './assets/styles/global.css'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(i18n)

interface SessionResponse {
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
function redactSession(session: SessionResponse): string {
  const rest: Record<string, unknown> = { ...session }
  delete rest.api_token
  return JSON.stringify(rest)
}

// dev 模式后端（go run）启动比 Vite 慢，窗口打开时后端可能还在编译；
// 轮询直到后端就绪并返回真实会话，避免把"后端未就绪"误判为"未登录"而停在登录页。
async function fetchSession(): Promise<SessionResponse> {
  const deadline = Date.now() + 30_000
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
      await new Promise(resolve => setTimeout(resolve, 1000))
    }
  }
}

function applySession(
  authStore: ReturnType<typeof useAuthStore>,
  appStore: ReturnType<typeof useAppStore>,
  session: SessionResponse,
) {
  console.log('[session] 应用会话结果', redactSession(session))
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

async function bootstrap() {
  // Before the first navigation of the route, restore the logged-in state with the backend HttpOnly Session as the only truth.
  const authStore = useAuthStore(pinia)
  const appStore = useAppStore(pinia)

  console.log(
    '[app] 前端启动 mode=' +
      import.meta.env.MODE +
      ' api_base_url=' +
      (import.meta.env.VITE_API_BASE_URL || '(空)'),
  )

  initializeLocale()
  startLocaleSync()

  // 先挂载页面再异步探测会话，避免启动等待导致白屏
  app.use(router)
  app.use(Antd)
  setApiFeedbackHandler(({ message: feedbackMessage }) => {
    message.warning(feedbackMessage)
  })
  app.mount('#app')

  try {
    applySession(authStore, appStore, await fetchSession())
  } catch (error) {
    console.log('[session] 启动探测 30 秒耗尽仍未成功', String(error))
    authStore.setLocalSession(false)
    authStore.clearCloudAuth()
    appStore.setCloudStatus('offline')
  }
}

void bootstrap()
