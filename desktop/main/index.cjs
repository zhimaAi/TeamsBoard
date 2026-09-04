'use strict'

const path = require('node:path')
const { app, dialog, Menu, session, Tray } = require('electron')
const { APP_ICON_PATH, configureAppIdentity } = require('./app-identity.cjs')
const { BackendSupervisor } = require('./backend-supervisor.cjs')
const { DesktopPreferences } = require('./desktop-preferences.cjs')
const { registerDialogIPC } = require('./ipc/dialog.cjs')
const { registerApiTokenIPC } = require('./ipc/api-token.cjs')
const { registerTrayIPC } = require('./ipc/tray.cjs')
const { installApplicationMenu } = require('./application-menu.cjs')
const { TrayManager } = require('./tray-manager.cjs')
const { createMainWindow } = require('./window-manager.cjs')

const repoRoot = path.resolve(__dirname, '..', '..')
const rendererURL = process.env.GOTEAMS_DESKTOP_RENDERER_URL || ''

// API 响应已由后端 Cache-Control: no-store 与前端 fetch cache: 'no-store' 保证实时性；
// 图片、文件等静态资源恢复走 Chromium 磁盘缓存。
configureAppIdentity(app)
installApplicationMenu(Menu)
const singleInstance = app.requestSingleInstanceLock()

let mainWindow = null
let backend = null
let backendConnection = null
let removeIPC = null
let removeApiTokenIPC = null
let removeTrayIPC = null
let desktopPreferences = null
let trayManager = null
let quitting = false
let exitCode = 0
let closeDecisionPending = false
let quitConfirmationPending = false

function failAndQuit(title, error) {
  if (quitting) return
  exitCode = 1
  dialog.showErrorBox(title, error instanceof Error ? error.message : String(error))
  app.quit()
}

function isMainWindowVisible() {
  return Boolean(mainWindow && !mainWindow.isDestroyed() && mainWindow.isVisible())
}

function refreshTray() {
  trayManager?.refresh()
}

function trackMainWindow(win) {
  mainWindow = win
  win.on('close', event => {
    if (quitting) return
    event.preventDefault()
    void handleMainWindowClose(win)
  })
  win.on('show', refreshTray)
  win.on('hide', refreshTray)
  win.on('minimize', refreshTray)
  win.on('restore', refreshTray)
  win.once('closed', () => {
    if (mainWindow === win) mainWindow = null
    refreshTray()
  })
}

function showOrCreateMainWindow() {
  if (mainWindow && !mainWindow.isDestroyed()) {
    if (mainWindow.isMinimized()) mainWindow.restore()
    mainWindow.show()
    mainWindow.focus()
    refreshTray()
    return
  }
  if (!quitting && backendConnection) {
    const created = createMainWindow({ ...backendConnection, browserTicket: '' })
    trackMainWindow(created.win)
  }
}

function hideMainWindow() {
  if (!mainWindow || mainWindow.isDestroyed()) return
  mainWindow.hide()
  refreshTray()
}

function showNativeDialog(options) {
  return isMainWindowVisible()
    ? dialog.showMessageBox(mainWindow, options)
    : dialog.showMessageBox(options)
}

async function requestQuit() {
  if (quitting || quitConfirmationPending) return

  const taskStatus = trayManager?.getTaskStatus()
  let dialogOptions = null
  if (!taskStatus?.known) {
    dialogOptions = {
      type: 'warning',
      title: '退出 TeamsBoard',
      message: '无法确认是否有任务正在执行',
      detail: '退出 TeamsBoard 将停止本地后台服务。建议继续在后台运行。',
      buttons: ['继续后台运行', '仍然退出'],
      defaultId: 0,
      cancelId: 0,
    }
  } else if (taskStatus.count > 0) {
    dialogOptions = {
      type: 'warning',
      title: '退出 TeamsBoard',
      message: `当前有 ${taskStatus.count} 个任务正在执行`,
      detail: '退出 TeamsBoard 将中断这些任务。',
      buttons: ['继续后台运行', '仍然退出'],
      defaultId: 0,
      cancelId: 0,
    }
  }

  if (!dialogOptions) {
    app.quit()
    return
  }

  quitConfirmationPending = true
  try {
    const result = await showNativeDialog(dialogOptions)
    if (result.response === 1) app.quit()
  } finally {
    quitConfirmationPending = false
  }
}

async function handleMainWindowClose(win) {
  if (quitting || closeDecisionPending || !desktopPreferences) return

  const preferences = desktopPreferences.get()
  if (preferences.closeTipShown) {
    if (preferences.closeBehavior === 'quit') {
      await requestQuit()
    } else if (!win.isDestroyed()) {
      hideMainWindow()
    }
    return
  }

  closeDecisionPending = true
  try {
    const result = await showNativeDialog({
      type: 'question',
      title: '关闭 TeamsBoard',
      message: '关闭窗口后，TeamsBoard 可以继续在后台运行',
      detail: '正在执行的任务不会中断，你可以通过系统托盘重新打开或退出应用。',
      buttons: ['最小化到托盘', '退出 TeamsBoard', '取消'],
      defaultId: 0,
      cancelId: 2,
    })
    if (result.response === 0) {
      try {
        desktopPreferences.setCloseBehavior('hide')
      } catch {
        // 保存偏好失败时仍优先保留后台任务，下一次关闭会再次提示。
      }
      if (!win.isDestroyed()) hideMainWindow()
    } else if (result.response === 1) {
      try {
        desktopPreferences.setCloseBehavior('quit')
      } catch {
        // 用户已明确选择退出，偏好保存失败不应阻止本次退出。
      }
      await requestQuit()
    }
  } finally {
    closeDecisionPending = false
  }
}

function createTray() {
  desktopPreferences = new DesktopPreferences(app.getPath('userData'))
  trayManager = new TrayManager({
    Tray,
    Menu,
    iconPath: APP_ICON_PATH,
    onShowWindow: showOrCreateMainWindow,
    onHideWindow: hideMainWindow,
    onQuit: () => void requestQuit(),
    isWindowVisible: isMainWindowVisible,
  })
  trayManager.create()
}

async function startApplication() {
  // 启动时兜底清一次历史磁盘缓存（旧版本曾缓存过 /api/local/* 的过期响应）；
  // 清理后静态资源恢复正常缓存
  try {
    await session.defaultSession.clearCache()
  } catch {
    // 清理失败不影响启动
  }
  if (quitting) return

  backend = new BackendSupervisor({
    isPackaged: app.isPackaged,
    resourcesPath: process.resourcesPath,
    repoRoot,
    rendererURL,
  })

  backend.on('exit', ({ expected }) => {
    if (!expected && !quitting) {
      failAndQuit('TeamsBoard 服务已停止', '本地服务意外退出，客户端即将关闭。')
    }
  })
  backend.on('error', error => {
    failAndQuit('TeamsBoard 服务启动失败', error)
  })

  backendConnection = await backend.start()
  if (quitting) return
  const created = createMainWindow(backendConnection)
  trackMainWindow(created.win)
  removeIPC = registerDialogIPC(created.allowedOrigins)
  removeApiTokenIPC = registerApiTokenIPC(created.allowedOrigins, backendConnection.apiToken)
  removeTrayIPC = registerTrayIPC(created.allowedOrigins, desktopPreferences, trayManager)
  trayManager.setReady()
}

if (!singleInstance) {
  // requestSingleInstanceLock has already notified the existing process. This
  // process has no window or sidecar to clean up and should terminate normally.
  app.quit()
} else {
  app.whenReady().then(() => {
    createTray()
    return startApplication()
  }).catch(error => {
    failAndQuit('TeamsBoard 启动失败', error)
  })

  app.on('second-instance', () => {
    showOrCreateMainWindow()
  })

  app.on('activate', () => {
    showOrCreateMainWindow()
  })

  app.on('before-quit', event => {
    if (quitting) return
    event.preventDefault()
    quitting = true
    removeIPC?.()
    removeApiTokenIPC?.()
    removeTrayIPC?.()
    trayManager?.destroy()
    void Promise.resolve(backend?.stop()).finally(() => app.exit(exitCode))
  })
}
