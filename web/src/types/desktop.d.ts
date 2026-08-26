export {}

declare global {
  interface Window {
    goteamsDesktop?: {
      readonly platform: string
      selectDirectories(options?: {
        defaultPath?: string
        multiple?: boolean
      }): Promise<string[]>
      /** 本次启动的本地 API 鉴权 token（由 Electron 主进程生成） */
      getApiToken(): Promise<string>
      getCloseBehavior(): Promise<{ closeBehavior: 'hide' | 'quit' }>
      setCloseBehavior(closeBehavior: 'hide' | 'quit'): Promise<{ closeBehavior: 'hide' | 'quit' }>
      setTrayStatus(status?: { runningTaskCount?: number }): void
    }
  }
}
