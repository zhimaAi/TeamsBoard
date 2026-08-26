<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  getDesktopCloseBehavior,
  setDesktopCloseBehavior,
  type DesktopCloseBehavior,
} from '@/composables/useDesktop'

const closeBehavior = ref<DesktopCloseBehavior>('hide')
const savedCloseBehavior = ref<DesktopCloseBehavior>('hide')
const loading = ref(true)
const saving = ref(false)
const errorMessage = ref('')

async function loadCloseBehavior() {
  loading.value = true
  errorMessage.value = ''
  try {
    const loadedCloseBehavior = await getDesktopCloseBehavior()
    closeBehavior.value = loadedCloseBehavior
    savedCloseBehavior.value = loadedCloseBehavior
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '读取桌面客户端设置失败'
  } finally {
    loading.value = false
  }
}

async function saveCloseBehavior() {
  const nextBehavior = closeBehavior.value
  saving.value = true
  errorMessage.value = ''
  try {
    const savedBehavior = await setDesktopCloseBehavior(nextBehavior)
    closeBehavior.value = savedBehavior
    savedCloseBehavior.value = savedBehavior
  } catch (error) {
    closeBehavior.value = savedCloseBehavior.value
    errorMessage.value = error instanceof Error ? error.message : '保存桌面客户端设置失败'
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void loadCloseBehavior()
})
</script>

<template>
  <a-card title="桌面客户端" :bordered="false" class="desktop-client-settings">
    <a-alert v-if="errorMessage" type="error" show-icon :message="errorMessage" class="desktop-client-settings__error">
      <template #action>
        <a-button type="link" size="small" @click="loadCloseBehavior">重试</a-button>
      </template>
    </a-alert>
    <a-spin :spinning="loading">
      <a-form layout="vertical">
        <a-form-item label="关闭主窗口时" extra="最小化到系统托盘后，正在执行的任务会继续运行。">
          <a-radio-group v-model:value="closeBehavior" :disabled="loading || saving" @change="saveCloseBehavior">
            <a-radio value="hide">最小化到系统托盘</a-radio>
            <a-radio value="quit">退出 TeamsBoard</a-radio>
          </a-radio-group>
        </a-form-item>
      </a-form>
    </a-spin>
  </a-card>
</template>

<style scoped>
.desktop-client-settings {
  max-width: 640px;
}

.desktop-client-settings__error {
  margin-bottom: 16px;
}
</style>
