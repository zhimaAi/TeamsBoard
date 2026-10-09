/**
 * Electron IPC 抛出的错误在渲染层会被包一层前缀：
 *   Error invoking remote method 'desktop:xxx': Error: 真正的错误信息
 * 直接展示会把内部通道名暴露给用户，这里统一剥掉外壳，只保留业务信息。
 */
const IPC_ERROR_PREFIX = /^Error invoking remote method '[^']*':\s*/

export function ipcErrorMessage(error: unknown, fallback: string): string {
  if (!(error instanceof Error)) return fallback
  const message = error.message.replace(IPC_ERROR_PREFIX, '').replace(/^Error:\s*/, '').trim()
  return message || fallback
}
