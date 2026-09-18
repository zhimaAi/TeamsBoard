import { t } from '@/i18n'

export function isDesktopRuntime(): boolean {
  return typeof window !== 'undefined' && Boolean(window.goteamsDesktop)
}

export type DesktopCloseBehavior = 'hide' | 'quit'

export interface DesktopTaskNotification {
  title: string
  body: string
  taskUuid: string
  stepUuid?: string
}

async function pickDirectories(
  defaultPath: string | undefined,
  multiple: boolean,
): Promise<string[]> {
  const bridge = window.goteamsDesktop
  if (!bridge) {
    throw new Error(t('components.errors.selectDirectoryUnsupported'))
  }
  return (await bridge.selectDirectories({ defaultPath, multiple })) ?? []
}

export async function selectDirectory(defaultPath?: string): Promise<string | undefined> {
  const paths = await pickDirectories(defaultPath, false)
  return paths[0]
}

export async function selectDirectories(defaultPath?: string): Promise<string[]> {
  return pickDirectories(defaultPath, true)
}

export async function openDirectory(directoryPath: string): Promise<void> {
  const bridge = window.goteamsDesktop
  if (!bridge) {
    throw new Error(t('components.errors.openDirectoryUnsupported'))
  }
  await bridge.openDirectory(directoryPath)
}

export async function openTerminal(directoryPath: string): Promise<void> {
  const bridge = window.goteamsDesktop
  if (!bridge?.openTerminal) throw new Error(t('workflows.task.progress.terminalUnsupported'))
  await bridge.openTerminal(directoryPath)
}

export async function openCodexThread(
  directoryPath: string,
  prompt: string,
  threadId?: string,
): Promise<void> {
  const bridge = window.goteamsDesktop
  if (!bridge) {
    throw new Error(t('components.errors.openCodexUnsupported'))
  }
  await bridge.openCodexThread({ directoryPath, prompt, threadId })
}

export async function getDesktopCloseBehavior(): Promise<DesktopCloseBehavior> {
  const bridge = window.goteamsDesktop
  if (!bridge) {
    throw new Error(t('components.errors.desktopSettingsUnsupported'))
  }
  return (await bridge.getCloseBehavior()).closeBehavior
}

export async function setDesktopCloseBehavior(
  closeBehavior: DesktopCloseBehavior,
): Promise<DesktopCloseBehavior> {
  const bridge = window.goteamsDesktop
  if (!bridge) {
    throw new Error(t('components.errors.desktopSettingsUnsupported'))
  }
  return (await bridge.setCloseBehavior(closeBehavior)).closeBehavior
}

export function syncTrayTaskCount(runningTaskCount: number) {
  window.goteamsDesktop?.setTrayStatus({ runningTaskCount })
}

export function showTaskNotification(notification: DesktopTaskNotification): Promise<{
  shown: boolean
  reason?: string
  message?: string
}> {
  return (
    window.goteamsDesktop?.showTaskNotification(notification) ??
    Promise.resolve({ shown: false, reason: 'unsupported' })
  )
}

export function onOpenTaskConversation(
  listener: (target: { taskUuid: string; stepUuid?: string }) => void,
) {
  return window.goteamsDesktop?.onOpenTaskConversation(listener) ?? (() => {})
}
