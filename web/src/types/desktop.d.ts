export {}

declare global {
  interface DesktopTaskNotificationPayload {
    title: string
    body: string
    taskUuid: string
    stepUuid?: string
  }

  interface Window {
    goteamsDesktop?: {
      readonly platform: string
      selectDirectories(options?: { defaultPath?: string; multiple?: boolean }): Promise<string[]>
      openDirectory(directoryPath: string): Promise<{ opened: true }>
      openTerminal(directoryPath: string): Promise<{ opened: true }>
      openCodexThread(payload: {
        directoryPath: string
        prompt: string
        threadId?: string
      }): Promise<{ opened: true }>
      /** 本次启动的本地 API 鉴权 token（由 Electron 主进程生成） */
      getApiToken(): Promise<string>
      getCloseBehavior(): Promise<{ closeBehavior: 'hide' | 'quit' }>
      setCloseBehavior(closeBehavior: 'hide' | 'quit'): Promise<{ closeBehavior: 'hide' | 'quit' }>
      /** 把当前应用语言同步给主进程，用于托盘与原生对话框 */
      setLocale(locale: string): Promise<{ locale: string }>
      setTrayStatus(status?: { runningTaskCount?: number }): void
      showTaskNotification(
        payload: DesktopTaskNotificationPayload,
      ): Promise<{ shown: boolean; reason?: string; message?: string }>
      onOpenTaskConversation(
        listener: (payload: { taskUuid: string; stepUuid?: string }) => void,
      ): () => void
    }
  }
}
