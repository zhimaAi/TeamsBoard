<script setup lang="ts">
import { ref, watch } from 'vue'

export interface LoginForm {
  serverType: 'official' | 'private'
  serverUrl: string
  account: string
  password: string
}

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (event: 'update:open', value: boolean): void
  (event: 'login', form: LoginForm): void
}>()

const serverType = ref<'official' | 'private'>('official')
const serverUrl = ref('')
const account = ref('')
const password = ref('')
const passwordVisible = ref(false)
const loading = ref(false)
const errorText = ref('')

function close() {
  emit('update:open', false)
}

function resetState() {
  serverType.value = 'official'
  serverUrl.value = ''
  account.value = ''
  password.value = ''
  passwordVisible.value = false
  loading.value = false
  errorText.value = ''
}

function togglePasswordVisible() {
  passwordVisible.value = !passwordVisible.value
}

async function handleLogin() {
  errorText.value = ''

  if (serverType.value === 'private' && !serverUrl.value.trim()) {
    errorText.value = '请输入私有化部署地址'
    return
  }
  if (!account.value.trim()) {
    errorText.value = '请输入账号'
    return
  }
  if (!password.value) {
    errorText.value = '请输入密码'
    return
  }

  loading.value = true
  try {
    emit('login', {
      serverType: serverType.value,
      serverUrl: serverUrl.value.trim(),
      account: account.value.trim(),
      password: password.value,
    })
  } finally {
    loading.value = false
  }
}

watch(() => props.open, (open) => {
  if (open) {
    resetState()
  }
})
</script>

<template>
  <Teleport to="body">
    <Transition name="login-fade">
      <div v-if="props.open" class="login-mask" @click.self="close">
        <div class="login-card" role="dialog" aria-modal="true" aria-label="登录">
          <!-- Logo -->
          <div class="login-logo">
            <span class="login-logo-text">GT</span>
          </div>

          <!-- Title -->
          <h2 class="login-title">欢迎回来</h2>
          <p class="login-subtitle">请登录以继续使用 Teams AI desk</p>

          <!-- Error hint -->
          <div v-if="errorText" class="login-error">
            {{ errorText }}
          </div>

          <!-- Form -->
          <form class="login-form" @submit.prevent="handleLogin">
            <!-- Server address -->
            <div class="login-field">
              <label class="login-label">服务器地址</label>
              <div class="login-radio-group">
                <label class="login-radio" :class="{ active: serverType === 'official' }">
                  <input v-model="serverType" type="radio" value="official">
                  <span class="login-radio-dot" />
                  <span class="login-radio-text">官方</span>
                </label>
                <label class="login-radio" :class="{ active: serverType === 'private' }">
                  <input v-model="serverType" type="radio" value="private">
                  <span class="login-radio-dot" />
                  <span class="login-radio-text">私有化部署</span>
                </label>
              </div>

              <div class="login-input-wrap">
                <svg class="login-input-icon" viewBox="0 0 20 20" width="18" height="18" fill="none" aria-hidden="true">
                  <circle cx="10" cy="10" r="8.5" stroke="currentColor" stroke-width="1.5" />
                  <ellipse cx="10" cy="10" rx="3.5" ry="8.5" stroke="currentColor" stroke-width="1.5" />
                  <path d="M1.5 10h17" stroke="currentColor" stroke-width="1.5" />
                </svg>
                <input
                  v-if="serverType === 'official'"
                  v-model="serverUrl"
                  type="text"
                  placeholder="goteams.cn"
                >
                <input
                  v-else
                  v-model="serverUrl"
                  type="text"
                  placeholder="请输入私有化部署地址，例如：https://t..."
                >
              </div>
            </div>

            <!-- Account -->
            <div class="login-field">
              <label class="login-label required">账号</label>
              <div class="login-input-wrap">
                <svg class="login-input-icon" viewBox="0 0 20 20" width="18" height="18" fill="none" aria-hidden="true">
                  <circle cx="10" cy="6" r="3.5" stroke="currentColor" stroke-width="1.5" />
                  <path d="M3 17c0-4.5 3.5-7 7-7s7 2.5 7 7" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
                </svg>
                <input v-model="account" type="text" placeholder="请输入账号">
              </div>
            </div>

            <!-- Password -->
            <div class="login-field">
              <label class="login-label required">密码</label>
              <div class="login-input-wrap">
                <svg class="login-input-icon" viewBox="0 0 20 20" width="18" height="18" fill="none" aria-hidden="true">
                  <rect x="3.5" y="7.5" width="13" height="10" rx="2" stroke="currentColor" stroke-width="1.5" />
                  <path d="M6 7.5V5a4 4 0 018 0v2.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
                </svg>
                <input v-model="password" :type="passwordVisible ? 'text' : 'password'" placeholder="请输入密码">
                <button
                  type="button"
                  class="login-eye"
                  :aria-label="passwordVisible ? '隐藏密码' : '显示密码'"
                  @click="togglePasswordVisible"
                >
                  <svg v-if="passwordVisible" viewBox="0 0 20 20" width="18" height="18" fill="none" aria-hidden="true">
                    <path d="M2 10s3-6 8-6 8 6 8 6-3 6-8 6-8-6-8-6z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                    <circle cx="10" cy="10" r="2.5" stroke="currentColor" stroke-width="1.5" />
                    <path d="M3.5 3.5l13 13" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
                  </svg>
                  <svg v-else viewBox="0 0 20 20" width="18" height="18" fill="none" aria-hidden="true">
                    <path d="M2 10s3-6 8-6 8 6 8 6-3 6-8 6-8-6-8-6z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                    <circle cx="10" cy="10" r="2.5" stroke="currentColor" stroke-width="1.5" />
                  </svg>
                </button>
              </div>
            </div>

            <!-- Login button -->
            <button type="submit" class="login-submit" :disabled="loading">
              {{ loading ? '登录中…' : '登录' }}
            </button>
          </form>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.login-mask {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgb(0 0 0 / 45%);
}

.login-card {
  width: 420px;
  max-width: 100%;
  padding: 40px 48px 48px;
  font-family: 'PingFang SC', 'Microsoft YaHei', sans-serif;
  text-align: center;
  background: #fff;
  border-radius: 16px;
  box-shadow: 0 12px 48px rgb(0 0 0 / 18%);
}

/* Logo */
.login-logo {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 64px;
  height: 64px;
  margin: 0 auto 20px;
  color: #fff;
  background: #2475fc;
  border-radius: 50%;
}
.login-logo-text {
  font-size: 24px;
  font-weight: 700;
  letter-spacing: 0.5px;
}

/* Title */
.login-title {
  margin: 0 0 8px;
  font-size: 22px;
  font-weight: 600;
  color: #262626;
}
.login-subtitle {
  margin: 0 0 28px;
  font-size: 14px;
  color: #8c8c8c;
}

/* Error hint */
.login-error {
  margin-bottom: 16px;
  padding: 8px 12px;
  font-size: 13px;
  color: #cf1322;
  text-align: left;
  background: #fff1f0;
  border: 1px solid #ffccc7;
  border-radius: 6px;
}

/* Form */
.login-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
  text-align: left;
}

.login-field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.login-label {
  font-size: 14px;
  color: #262626;
}
.login-label.required::before {
  margin-right: 4px;
  color: #fb363f;
  content: '*';
}

/* Radio */
.login-radio-group {
  display: flex;
  align-items: center;
  gap: 20px;
}
.login-radio {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: #595959;
  cursor: pointer;
}
.login-radio input {
  position: absolute;
  width: 0;
  height: 0;
  opacity: 0;
}
.login-radio-dot {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  background: #fff;
  border: 1px solid #d9d9d9;
  border-radius: 50%;
  transition: border-color 0.2s;
}
.login-radio-dot::after {
  width: 8px;
  height: 8px;
  content: '';
  background: #2475fc;
  border-radius: 50%;
  opacity: 0;
  transform: scale(0);
  transition: opacity 0.2s, transform 0.2s;
}
.login-radio.active .login-radio-dot {
  border-color: #2475fc;
}
.login-radio.active .login-radio-dot::after {
  opacity: 1;
  transform: scale(1);
}
.login-radio.active .login-radio-text {
  color: #262626;
}

/* Input box */
.login-input-wrap {
  position: relative;
  display: flex;
  align-items: center;
  height: 40px;
  padding: 0 12px;
  background: #fff;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  transition: border-color 0.2s;
}
.login-input-wrap:focus-within {
  border-color: #2475fc;
}
.login-input-icon {
  flex: none;
  margin-right: 10px;
  color: #8594aa;
}
.login-input-wrap input {
  flex: 1;
  min-width: 0;
  height: 100%;
  font-size: 14px;
  color: #262626;
  background: transparent;
  border: 0;
  outline: none;
}
.login-input-wrap input::placeholder {
  color: rgb(0 0 0 / 25%);
}

/* Eye button */
.login-eye {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  margin-left: 4px;
  color: #8594aa;
  background: transparent;
  border: 0;
  border-radius: 4px;
  cursor: pointer;
  transition: color 0.2s, background 0.2s;
}
.login-eye:hover {
  color: #595959;
  background: #f5f5f5;
}

/* Login button */
.login-submit {
  width: 100%;
  height: 40px;
  margin-top: 4px;
  font-size: 15px;
  font-weight: 500;
  color: #fff;
  background: #2475fc;
  border: 0;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.2s;
}
.login-submit:hover {
  background: #1c66e0;
}
.login-submit:disabled {
  color: rgb(0 0 0 / 25%);
  background: #f5f5f5;
  cursor: not-allowed;
}

.login-fade-enter-active,
.login-fade-leave-active {
  transition: opacity 0.2s ease;
}
.login-fade-enter-from,
.login-fade-leave-to {
  opacity: 0;
}
</style>
