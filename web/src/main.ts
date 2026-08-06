import { createApp } from 'vue'
import { createPinia } from 'pinia'
import Antd from 'ant-design-vue'
import App from './App.vue'
import router from './router'
import apiClient from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import './assets/styles/global.css'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)

async function bootstrap() {
  // Before the first navigation of the route, restore the logged-in state with the backend HttpOnly Session as the only truth.
  const authStore = useAuthStore(pinia)
  const appStore = useAppStore(pinia)
  try {
    const session = await apiClient.get<{
      local_authenticated: boolean
      cloud_logged_in: boolean
      user?: {
        id: string
        username: string
        display_name?: string
        avatar?: string
        role?: string
      }
    }>('/auth/session')
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
  } catch {
    authStore.setLocalSession(false)
    authStore.clearCloudAuth()
    appStore.setCloudStatus('offline')
  }

  app.use(router)
  app.use(Antd)
  app.mount('#app')
}

void bootstrap()
