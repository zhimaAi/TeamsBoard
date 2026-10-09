<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Modal, message } from 'ant-design-vue'
import apiClient from '@/api/client'
import { useLocalWS } from '@/composables/useLocalWebSocket'
import { useAppI18n } from '@/i18n'

type ChannelStatus = 'disconnected' | 'connecting' | 'connected'
type LoginStatus = 'waiting' | 'scanned' | 'verifying' | 'need_verifycode' | 'expired' | 'error' | 'connected'

interface WeChatStatus {
  status: ChannelStatus
  connected_at: number
  login_id: string
  last_error: string
  reply_message_id_missing: boolean
}

interface LoginAttempt {
  id: string
  status: LoginStatus
  qr_image: string
  error?: string
}

interface UncertainRun {
  message_id: string
  task_uuid: string
  task_title: string
  received_at: number
}

const { t, d } = useAppI18n()
const status = ref<ChannelStatus>('disconnected')
const lastError = ref('')
const replyMessageIDMissing = ref(false)
const loading = ref(true)
const loadError = ref(false)
const loginOpen = ref(false)
const login = ref<LoginAttempt | null>(null)
const loginBusy = ref(false)
const verifyBusy = ref(false)
const verifyCode = ref('')
const uncertainRuns = ref<UncertainRun[]>([])
const uncertainLoading = ref(false)
const uncertainError = ref(false)
const acknowledgeBusy = ref<string | null>(null)
let loginTimer: ReturnType<typeof setInterval> | undefined
let statusTimer: ReturnType<typeof setInterval> | undefined
let uncertainRequestVersion = 0
let statusRequestVersion = 0

const cards = computed(() => [
  { key: 'wechat', name: t('remoteChannels.wechat'), description: t('remoteChannels.wechatDescription'), mark: 'wechat', glyph: '' },
  { key: 'wecom', name: t('remoteChannels.wecom'), description: t('remoteChannels.wecomDescription'), mark: 'wecom', glyph: '企' },
  { key: 'feishu', name: t('remoteChannels.feishu'), description: t('remoteChannels.feishuDescription'), mark: 'feishu', glyph: '飞' },
  { key: 'dingtalk', name: t('remoteChannels.dingtalk'), description: t('remoteChannels.dingtalkDescription'), mark: 'dingtalk', glyph: '钉' },
  { key: 'app', name: t('remoteChannels.app'), description: t('remoteChannels.appDescription'), mark: 'app', glyph: '▣' },
])

const loginStatusText = computed(() => {
  switch (login.value?.status) {
    case 'scanned': return t('remoteChannels.scanned')
    case 'verifying': return t('remoteChannels.verifying')
    case 'need_verifycode': return t('remoteChannels.needVerify')
    case 'expired': return t('remoteChannels.expired')
    case 'error': return login.value.error || t('remoteChannels.connectFailed')
    default: return t('remoteChannels.waiting')
  }
})

const statusErrorText = computed(() => lastError.value === 'wechat_session_expired'
  ? t('remoteChannels.connectionLost')
  : t('remoteChannels.statusError'))

function stopLoginPolling() {
  if (loginTimer) clearInterval(loginTimer)
  loginTimer = undefined
}

async function loadStatus(silent = false) {
  const requestVersion = ++statusRequestVersion
  if (!silent) loading.value = true
  try {
    const response = await apiClient.get<WeChatStatus>('/remote-channels/wechat')
    if (!response || !['disconnected', 'connecting', 'connected'].includes(response.status)) {
      throw new Error('Invalid channel status')
    }
    if (requestVersion !== statusRequestVersion) return
    status.value = response.status
    lastError.value = response.last_error
    replyMessageIDMissing.value = response.reply_message_id_missing === true
    loadError.value = false
  } catch {
    if (requestVersion === statusRequestVersion) loadError.value = true
  } finally {
    if (requestVersion === statusRequestVersion) loading.value = false
  }
}

useLocalWS('remote.wechat.status', () => { void loadStatus(true) })

async function loadUncertain() {
  const requestVersion = ++uncertainRequestVersion
  uncertainLoading.value = true
  uncertainError.value = false
  try {
    const response = await apiClient.get<{ items: UncertainRun[] }>('/remote-channels/wechat/uncertain')
    if (!response || !Array.isArray(response.items)) throw new Error('Invalid uncertain runs')
    if (requestVersion === uncertainRequestVersion) uncertainRuns.value = response.items
  } catch {
    if (requestVersion === uncertainRequestVersion) uncertainError.value = true
  } finally {
    if (requestVersion === uncertainRequestVersion) uncertainLoading.value = false
  }
}

function acknowledgeUncertain(run: UncertainRun) {
  if (acknowledgeBusy.value) return
  Modal.confirm({
    title: t('remoteChannels.uncertainConfirmTitle'),
    content: t('remoteChannels.uncertainConfirmDescription', { task: run.task_title || run.task_uuid || t('remoteChannels.uncertainTaskUnknown') }),
    okText: t('remoteChannels.uncertainAcknowledge'),
    okType: 'danger',
    onOk: async () => {
      acknowledgeBusy.value = run.message_id
      try {
        await apiClient.post('/remote-channels/wechat/uncertain/ack', { message_id: run.message_id })
        uncertainRuns.value = uncertainRuns.value.filter((item) => item.message_id !== run.message_id)
        await loadUncertain()
        message.success(t('remoteChannels.uncertainAcknowledged'))
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('remoteChannels.uncertainAckFailed'))
        throw error
      } finally {
        acknowledgeBusy.value = null
      }
    },
  })
}

async function pollLogin() {
  const id = login.value?.id
  if (!id || loginBusy.value) return
  try {
    const response = await apiClient.get<LoginAttempt>(`/remote-channels/wechat/login/${encodeURIComponent(id)}`)
    if (!response || response.id !== id || typeof response.status !== 'string') {
      throw new Error('Invalid login status')
    }
    if (login.value?.id !== id) return
    login.value = response
    if (response.status === 'connected') {
      stopLoginPolling()
      loginOpen.value = false
      status.value = 'connected'
      lastError.value = ''
      await loadStatus()
    } else if (response.status === 'expired' || response.status === 'error') {
      stopLoginPolling()
    }
  } catch {
    stopLoginPolling()
    if (login.value?.id === id) login.value = { ...login.value, status: 'error' }
  }
}

async function startLogin() {
  if (loginBusy.value) return
  loginBusy.value = true
  stopLoginPolling()
  verifyCode.value = ''
  login.value = null
  loginOpen.value = true
  try {
    const response = await apiClient.post<LoginAttempt>('/remote-channels/wechat/login', {})
    if (!response || !response.id || !response.qr_image) throw new Error(t('remoteChannels.connectFailed'))
    login.value = response
    status.value = 'connecting'
    loginTimer = setInterval(() => { void pollLogin() }, 2000)
  } catch (error) {
    login.value = { id: '', status: 'error', qr_image: '', error: error instanceof Error ? error.message : t('remoteChannels.connectFailed') }
  } finally {
    loginBusy.value = false
  }
}

async function submitVerification() {
  if (!login.value?.id || !verifyCode.value.trim() || verifyBusy.value) return
  verifyBusy.value = true
  try {
    await apiClient.post(`/remote-channels/wechat/login/${encodeURIComponent(login.value.id)}/verify`, { code: verifyCode.value.trim() })
    verifyCode.value = ''
    login.value = { ...login.value, status: 'verifying' }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('remoteChannels.verifyFailed'))
  } finally {
    verifyBusy.value = false
  }
}

function closeLogin() {
  loginOpen.value = false
  void loadStatus()
}

function disconnect() {
  Modal.confirm({
    title: t('remoteChannels.disconnectTitle'),
    content: t('remoteChannels.disconnectDescription'),
    okText: t('remoteChannels.disconnect'),
    okType: 'danger',
    onOk: async () => {
      try {
        const result = await apiClient.delete<{ status: ChannelStatus; wechat_notified: boolean }>('/remote-channels/wechat')
        statusRequestVersion++
        status.value = 'disconnected'
        lastError.value = ''
        replyMessageIDMissing.value = false
        login.value = null
        stopLoginPolling()
        uncertainRequestVersion++
        uncertainLoading.value = false
        await loadUncertain()
        if (result.wechat_notified) {
          message.success(t('remoteChannels.disconnectSuccess'))
        } else {
          message.warning(t('remoteChannels.stopNotifyFailed'))
        }
      } catch (error) {
        message.error(error instanceof Error ? error.message : t('remoteChannels.disconnectFailed'))
        throw error
      }
    },
  })
}

onMounted(() => {
  void loadStatus()
  void loadUncertain()
  statusTimer = setInterval(() => { void loadStatus(true) }, 10_000)
})
onUnmounted(() => {
  stopLoginPolling()
  if (statusTimer) clearInterval(statusTimer)
  statusRequestVersion++
  uncertainRequestVersion++
})
</script>

<template>
  <section class="remote-page">
    <header class="page-header"><h1>{{ t('remoteChannels.title') }}</h1></header>
    <div class="remote-content">
      <div class="intro-banner" role="note">
        <span class="info-symbol" aria-hidden="true"><svg width="12" height="12" viewBox="0 0 12 12" fill="none"><path d="M6 5v3.5M6 3.5h.01" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg></span>
        <div><strong>{{ t('remoteChannels.intro') }}</strong><p>{{ t('remoteChannels.hint') }}</p></div>
      </div>

      <div v-if="loading" class="state-message">{{ t('remoteChannels.loading') }}</div>
      <div v-if="loadError" class="state-message">
        {{ t('remoteChannels.loadFailed') }}
        <a-button size="small" @click="loadStatus()">{{ t('remoteChannels.retry') }}</a-button>
      </div>
      <a-alert v-if="lastError" class="status-alert" type="warning" show-icon :message="statusErrorText" />
      <div class="channel-grid">
          <article v-for="card in cards" :key="card.key" class="channel-card">
            <div class="card-heading">
              <span class="channel-icon" :class="card.mark" aria-hidden="true">
                <svg v-if="card.mark === 'wechat'" viewBox="0 0 36 36" fill="none">
                  <ellipse cx="15" cy="15" rx="12" ry="9.5" fill="white" />
                  <path d="m8 23-1 5 5-3" fill="white" />
                  <ellipse cx="23.5" cy="21" rx="9.5" ry="7.5" fill="#d9f6e6" />
                  <path d="m28 27 1 4-4-3" fill="#d9f6e6" />
                  <circle cx="11" cy="14" r="1.1" fill="#16a36a" /><circle cx="19" cy="14" r="1.1" fill="#16a36a" />
                </svg>
                <span v-else>{{ card.glyph }}</span>
              </span>
              <h2>{{ card.name }}</h2>
              <span v-if="card.key === 'wechat'" class="wechat-action-wrap">
                <a-button class="wechat-action" :class="{ 'wechat-action-with-warning': status === 'connected' && replyMessageIDMissing }" size="small" :loading="loginBusy" :disabled="loading || loadError" @click="status === 'connected' ? disconnect() : startLogin()">
                  {{ status === 'connected' ? t('remoteChannels.disconnect') : status === 'connecting' ? t('remoteChannels.connecting') : t('remoteChannels.connect') }}
                </a-button>
                <a-popover v-if="status === 'connected' && replyMessageIDMissing" :trigger="['hover', 'click']" placement="topRight">
                  <template #content><span class="wechat-reply-warning-text">{{ t('remoteChannels.replyMessageIDMissing') }}</span></template>
                  <button type="button" class="wechat-reply-warning" :aria-label="t('remoteChannels.replyMessageIDMissing')" />
                </a-popover>
              </span>
              <a-button v-if="card.key !== 'wechat'" class="channel-action" size="small" disabled>{{ t('remoteChannels.comingSoon') }}</a-button>
            </div>
            <p>{{ card.description }}</p>
          </article>
      </div>
      <section v-if="status === 'connected' || uncertainRuns.length || uncertainError || uncertainLoading" class="uncertain-panel" aria-labelledby="uncertain-title">
        <div class="uncertain-heading">
          <h2 id="uncertain-title">{{ t('remoteChannels.uncertainTitle') }}</h2>
          <a-button size="small" :loading="uncertainLoading" @click="loadUncertain">{{ t('remoteChannels.retry') }}</a-button>
        </div>
        <a-alert v-if="uncertainRuns.length" type="warning" show-icon :message="t('remoteChannels.uncertainDescription')" />
        <p v-if="uncertainError" class="uncertain-error">{{ t('remoteChannels.uncertainLoadFailed') }}</p>
        <p v-if="uncertainLoading && !uncertainRuns.length" class="uncertain-empty">{{ t('remoteChannels.uncertainLoading') }}</p>
        <p v-else-if="!uncertainRuns.length && !uncertainError" class="uncertain-empty">{{ t('remoteChannels.uncertainEmpty') }}</p>
        <div v-for="run in uncertainRuns" :key="run.message_id" class="uncertain-item">
          <div class="uncertain-details">
            <strong>{{ run.task_title || t('remoteChannels.uncertainTaskMissing') }}</strong>
            <span>{{ t('remoteChannels.uncertainMeta', { time: d(new Date(run.received_at), 'long'), id: run.task_uuid || t('remoteChannels.uncertainTaskUnknown') }) }}</span>
          </div>
          <router-link v-if="run.task_title" :to="{ name: 'workflows-task-detail', params: { taskUuid: run.task_uuid } }">
            {{ t('remoteChannels.openTask') }}
          </router-link>
          <a-button danger :loading="acknowledgeBusy === run.message_id" :disabled="!!acknowledgeBusy" @click="acknowledgeUncertain(run)">
            {{ t('remoteChannels.uncertainAcknowledge') }}
          </a-button>
        </div>
      </section>
    </div>

    <a-modal v-model:open="loginOpen" :title="t('remoteChannels.loginTitle')" :footer="null" :mask-closable="false" width="400px" @cancel="closeLogin">
      <div class="login-body">
        <p class="scan-prompt">{{ t('remoteChannels.scanPrompt') }}</p>
        <div class="qr-frame">
          <img v-if="login?.qr_image" :src="login.qr_image" :alt="t('remoteChannels.scanPrompt')">
          <a-spin v-else :spinning="loginBusy" />
        </div>
        <p class="login-state" :class="{ error: login?.status === 'error' || login?.status === 'expired' }" role="status">{{ loginStatusText }}</p>
        <div v-if="login?.status === 'need_verifycode'" class="verify-row">
          <a-input v-model:value="verifyCode" :placeholder="t('remoteChannels.verifyPlaceholder')" :aria-label="t('remoteChannels.verifyPlaceholder')" @press-enter="submitVerification" />
          <a-button type="primary" :loading="verifyBusy" @click="submitVerification">{{ t('remoteChannels.verify') }}</a-button>
        </div>
        <a-button v-if="login?.status === 'expired' || login?.status === 'error'" @click="startLogin">{{ t('remoteChannels.refreshQR') }}</a-button>
        <a-button class="close-button" @click="closeLogin">{{ t('remoteChannels.close') }}</a-button>
      </div>
    </a-modal>
  </section>
</template>

<style scoped>
.remote-page { height: 100%; min-height: 0; overflow: auto; background: #fff; color: #262626; }
.page-header { height: 64px; display: flex; align-items: center; padding: 0 24px; border-bottom: 1px solid #f0f0f0; }
.page-header h1 { margin: 0; font-size: 20px; line-height: 28px; font-weight: 600; }
.remote-content { padding: 24px; }
.intro-banner { display: flex; gap: 12px; align-items: flex-start; padding: 16px; background: #eaf0ff; border-radius: 12px; font-size: 14px; line-height: 22px; }
.intro-banner strong { font-weight: 600; }
.intro-banner p { margin: 4px 0 0; color: #595959; }
.info-symbol { display: inline-grid; place-items: center; width: 18px; height: 18px; flex: 0 0 18px; margin-top: 2px; border-radius: 50%; background: #3157e2; color: #fff; font-size: 12px; font-weight: 700; }
.state-message { padding: 24px 0; display: flex; align-items: center; gap: 12px; color: #595959; }
.status-alert { margin-top: 16px; }
.uncertain-panel { margin-top: 24px; padding: 20px; border: 1px solid #d9d9d9; border-radius: 12px; background: #fff; }
.uncertain-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.uncertain-heading h2 { margin: 0; font-size: 16px; line-height: 24px; font-weight: 600; }
.uncertain-error { margin: 12px 0 0; color: #fb363f; font-size: 14px; line-height: 22px; }
.uncertain-empty { margin: 12px 0 0; color: #8c8c8c; font-size: 14px; line-height: 22px; }
.uncertain-item { display: flex; align-items: center; gap: 16px; padding: 16px 0; border-bottom: 1px solid #f0f0f0; }
.uncertain-item:last-child { border-bottom: 0; padding-bottom: 0; }
.uncertain-details { display: flex; flex: 1; min-width: 0; flex-direction: column; gap: 4px; }
.uncertain-details strong { font-size: 14px; line-height: 22px; }
.uncertain-details span { color: #8c8c8c; font-size: 12px; line-height: 20px; overflow-wrap: anywhere; }
.channel-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; margin-top: 16px; }
.channel-card { min-width: 0; padding: 20px; border: 1px solid #e5e7eb; border-radius: 14px; background: #fff; }
.card-heading { display: flex; align-items: center; gap: 12px; }
.card-heading h2 { min-width: 0; margin: 0; flex: 1; font-size: 16px; font-weight: 600; line-height: 24px; }
.channel-card p { margin: 16px 0 0; color: #595959; font-size: 14px; line-height: 22px; }
.channel-action { flex: 0 0 auto; }
.wechat-action-wrap { position: relative; display: inline-flex; flex: 0 0 auto; }
.wechat-action-with-warning { padding-right: 28px; }
.wechat-reply-warning { position: absolute; top: 50%; right: 1px; display: inline-flex; align-items: center; justify-content: center; width: 24px; height: 24px; padding: 0; border: 0; background: transparent; cursor: pointer; transform: translateY(-50%); }
.wechat-reply-warning::before { display: inline-flex; align-items: center; justify-content: center; width: 15px; height: 15px; border-radius: 50%; background: #fb363f; color: #fff; content: '!'; font-size: 11px; font-weight: 700; }
.wechat-reply-warning:focus-visible { outline: 2px solid #3157e2; outline-offset: 2px; }
.wechat-reply-warning-text { display: block; max-width: 280px; }
.wechat-action:not(:disabled) { color: #3157e2; border-color: #3157e2; }
.channel-icon { display: inline-grid; place-items: center; width: 40px; height: 40px; flex: 0 0 40px; border-radius: 12px; color: white; font-size: 18px; font-weight: 700; }
.channel-icon svg { width: 28px; height: 28px; }
.wechat { background: #16a36a; }.wecom { background: #35a8ed; }.feishu { background: #4b79f5; }.dingtalk { background: #2783ee; }.app { background: #5b7ce7; }
.login-body { display: flex; align-items: center; flex-direction: column; padding: 12px 0 0; }
.scan-prompt { margin: 8px 0 16px; font-size: 14px; }
.qr-frame { display: grid; place-items: center; width: 240px; height: 240px; border: 1px solid #e5e7eb; border-radius: 12px; }
.qr-frame img { width: 224px; height: 224px; }
.login-state { min-height: 22px; margin: 16px 0; color: #595959; text-align: center; }
.login-state.error { color: #c63d43; }
.verify-row { display: flex; width: 100%; gap: 8px; margin-bottom: 12px; }
.close-button { align-self: flex-end; margin-top: 12px; }
@media (max-width: 820px) { .channel-grid { grid-template-columns: 1fr; } .remote-content { padding: 16px; } .uncertain-item { align-items: flex-start; flex-direction: column; } }
</style>
