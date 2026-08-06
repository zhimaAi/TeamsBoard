<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { LockOutlined, UserOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import { useLastLogin } from '@/composables/useLastLogin'

const props = defineProps<{
  loading: boolean
  /** Official cloud address (from config.ini api_base_url); the official option is disabled when empty */
  officialUrl?: string
}>()

const emit = defineEmits<{
  submit: [form: { serverType: 'official' | 'custom'; serverUrl: string; username: string; password: string }]
}>()

const serverType = ref<'official' | 'custom'>('official')
const form = reactive({
  serverUrl: '',
  username: '',
  password: '',
})

// In official mode the input shows the official address (read-only); switching to custom keeps the user's input.
watch([() => props.officialUrl, serverType], ([url, type]) => {
  if (type === 'official') {
    form.serverUrl = url ?? ''
  }
}, { immediate: true })

// Restore the last login method: select the corresponding option and, in custom mode, refill the last domain.
const lastLogin = useLastLogin().getLastLogin()
if (lastLogin) {
  serverType.value = lastLogin.serverType
  if (lastLogin.serverType === 'custom' && lastLogin.serverUrl) {
    form.serverUrl = lastLogin.serverUrl
  }
}

/** Force custom mode when the official address is empty */
function ensureServerType() {
  if (serverType.value === 'official' && !props.officialUrl?.trim()) {
    serverType.value = 'custom'
  }
}

function handleSubmit() {
  if (props.loading) return

  ensureServerType()

  const serverUrl = serverType.value === 'official' ? (props.officialUrl ?? '').trim() : form.serverUrl.trim()
  if (serverType.value === 'custom' && !serverUrl) {
    message.warning('请输入自定义云端地址')
    return
  }
  if (!serverType.value || (serverType.value === 'official' && !serverUrl)) {
    message.warning('官方云端地址未配置，请选择自定义地址')
    return
  }

  const username = form.username.trim()
  if (!username || !form.password) {
    message.warning('请输入账号和密码')
    return
  }

  emit('submit', {
    serverType: serverType.value,
    serverUrl,
    username,
    password: form.password,
  })
}
</script>

<template>
  <a-form :model="form" class="login-form" @finish="handleSubmit">
    <!-- Server address -->
    <a-form-item name="serverType">
      <div class="server-type-row">
        <label
          class="server-type-option"
          :class="{ active: serverType === 'official', disabled: !officialUrl?.trim() }"
        >
          <input
            v-model="serverType"
            type="radio"
            value="official"
            :disabled="!officialUrl?.trim()"
          >
          <span class="radio-dot" />
          <span class="radio-text">官方</span>
        </label>
        <label class="server-type-option" :class="{ active: serverType === 'custom' }">
          <input v-model="serverType" type="radio" value="custom">
          <span class="radio-dot" />
          <span class="radio-text">自定义地址</span>
        </label>
      </div>

      <a-input
        v-model:value="form.serverUrl"
        :readonly="serverType === 'official'"
        :disabled="loading"
        :placeholder="serverType === 'official'
          ? (officialUrl?.trim() ? '官方地址（来自 config.ini）' : '未配置官方地址，请选择自定义地址')
          : '请输入自定义云端地址，例如：https://goteams.example.com'"
        size="large"
      >
        <template #prefix>
          <svg viewBox="0 0 20 20" width="16" height="16" fill="none" aria-hidden="true">
            <circle cx="10" cy="10" r="8.5" stroke="currentColor" stroke-width="1.5" />
            <ellipse cx="10" cy="10" rx="3.5" ry="8.5" stroke="currentColor" stroke-width="1.5" />
            <path d="M1.5 10h17" stroke="currentColor" stroke-width="1.5" />
          </svg>
        </template>
      </a-input>
    </a-form-item>

    <a-form-item name="username">
      <a-input
        v-model:value="form.username"
        autocomplete="username"
        :disabled="loading"
        placeholder="账号"
        size="large"
      >
        <template #prefix><UserOutlined /></template>
      </a-input>
    </a-form-item>

    <a-form-item name="password">
      <a-input-password
        v-model:value="form.password"
        autocomplete="current-password"
        :disabled="loading"
        placeholder="密码"
        size="large"
      >
        <template #prefix><LockOutlined /></template>
      </a-input-password>
    </a-form-item>

    <a-form-item class="submit-item">
      <a-button type="primary" html-type="submit" size="large" :loading="loading" block>
        登录
      </a-button>
    </a-form-item>
  </a-form>
</template>

<style scoped>
.login-form {
  width: 100%;
  margin-top: 38px;
}

.login-form :deep(.ant-form-item) {
  margin-bottom: 20px;
}

.login-form :deep(.ant-input-affix-wrapper) {
  min-height: 50px;
  padding: 0 18px;
  border-color: #d6dce5;
  border-radius: 5px;
  box-shadow: none;
}

.login-form :deep(.ant-input-affix-wrapper-focused),
.login-form :deep(.ant-input-affix-wrapper:hover) {
  border-color: #3157e2;
}

.login-form :deep(.ant-input-prefix) {
  margin-inline-end: 12px;
  color: #a9b2c0;
}

.login-form :deep(.ant-input) {
  color: #303846;
  font-size: 15px;
}

.login-form :deep(.ant-input::placeholder) {
  color: #a9b2c0;
}

.login-form .submit-item {
  margin-top: 2px;
  margin-bottom: 0;
}

.login-form :deep(.ant-btn) {
  height: 55px;
  border-radius: 8px;
  font-size: 17px;
  font-weight: 600;
}

/* Official / custom radio */
.server-type-row {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-bottom: 10px;
}

.server-type-option {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: #595959;
  cursor: pointer;
  user-select: none;
}

.server-type-option.disabled {
  color: #bfbfbf;
  cursor: not-allowed;
}

.server-type-option input {
  position: absolute;
  width: 0;
  height: 0;
  opacity: 0;
}

.radio-dot {
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

.radio-dot::after {
  width: 8px;
  height: 8px;
  content: '';
  background: #3157e2;
  border-radius: 50%;
  opacity: 0;
  transform: scale(0);
  transition: opacity 0.2s, transform 0.2s;
}

.server-type-option.active .radio-dot {
  border-color: #3157e2;
}

.server-type-option.active .radio-dot::after {
  opacity: 1;
  transform: scale(1);
}

.server-type-option.active .radio-text {
  color: #262626;
}

.server-type-option.disabled .radio-text {
  color: #bfbfbf;
}
</style>
