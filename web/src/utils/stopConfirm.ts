const STOP_CONFIRM_SUPPRESSED_KEY = 'goteams.task.stop-confirm-suppressed'

export function isStopConfirmSuppressed(): boolean {
  try {
    return window.localStorage.getItem(STOP_CONFIRM_SUPPRESSED_KEY) === 'true'
  } catch {
    return false
  }
}

export function suppressStopConfirm(): void {
  try {
    window.localStorage.setItem(STOP_CONFIRM_SUPPRESSED_KEY, 'true')
  } catch {
    // 本地存储不可用时仅保留本次操作，不影响停止任务本身。
  }
}
