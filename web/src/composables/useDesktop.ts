export function isDesktopRuntime(): boolean {
  return typeof window !== 'undefined' && Boolean(window.goteamsDesktop)
}

export type DesktopCloseBehavior = 'hide' | 'quit'

async function pickDirectories(defaultPath: string | undefined, multiple: boolean): Promise<string[]> {
  const bridge = window.goteamsDesktop
  if (!bridge) {
    throw new Error('当前环境不支持选择本地文件夹，请使用 TeamsBoard 桌面客户端')
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

export async function getDesktopCloseBehavior(): Promise<DesktopCloseBehavior> {
  const bridge = window.goteamsDesktop
  if (!bridge) {
    throw new Error('当前环境不支持桌面客户端设置')
  }
  return (await bridge.getCloseBehavior()).closeBehavior
}

export async function setDesktopCloseBehavior(closeBehavior: DesktopCloseBehavior): Promise<DesktopCloseBehavior> {
  const bridge = window.goteamsDesktop
  if (!bridge) {
    throw new Error('当前环境不支持桌面客户端设置')
  }
  return (await bridge.setCloseBehavior(closeBehavior)).closeBehavior
}

export function syncTrayTaskCount(runningTaskCount: number) {
  window.goteamsDesktop?.setTrayStatus({ runningTaskCount })
}
