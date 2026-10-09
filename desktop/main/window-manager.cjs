'use strict'

const path = require('node:path')
const { app, BrowserWindow, session } = require('electron')
const { APP_ICON_PATH, APP_NAME } = require('./app-identity.cjs')
const { createOriginAllowlist, installNavigationPolicy, installAssetRefererPolicy } = require('./security.cjs')

// DevTools（F12）仅在非打包的开发模式启用，避免影响生产发布。
const isDev = !app.isPackaged

function createMainWindow({ baseURL, browserTicket, rendererURL }) {
  const contentBaseURL = rendererURL || baseURL
  const allowedOrigins = createOriginAllowlist([baseURL, rendererURL])
  const win = new BrowserWindow({
    width: 1440,
    height: 900,
    minWidth: 1080,
    minHeight: 680,
    show: false,
    autoHideMenuBar: true,
    // macOS 继续用 hiddenInset，红黄绿由系统画在内容区顶部。
    // Windows 用标题栏覆盖，把侧栏收起按钮放进系统按钮同一行，避免标题栏下面再空出一栏。
    // 窗口按钮仍由系统绘制，页面不画假按钮。
    ...(process.platform === 'darwin'
      ? { titleBarStyle: 'hiddenInset' }
      : process.platform === 'win32'
        ? {
            titleBarStyle: 'hidden',
            titleBarOverlay: {
              color: '#ffffff',
              symbolColor: '#1d1d1f',
              height: 38,
            },
          }
        : {}),
    icon: APP_ICON_PATH,
    backgroundColor: '#f5f7fb',
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'index.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      webviewTag: false,
    },
  })

  win.on('page-title-updated', event => event.preventDefault())
  win.setTitle(`${APP_NAME} v${app.getVersion()}`)

  installNavigationPolicy(win, allowedOrigins)
  installAssetRefererPolicy(session.defaultSession, win.webContents.id)
  session.defaultSession.setPermissionRequestHandler((_webContents, permission, callback) => {
    callback(permission === 'clipboard-sanitized-write')
  })

  if (isDev) {
    // 开发模式自动打开 DevTools，并支持按 F12 切换
    win.webContents.on('before-input-event', (event, input) => {
      if (input.key === 'F12') {
        event.preventDefault()
        if (win.webContents.isDevToolsOpened()) {
          win.webContents.closeDevTools()
        } else {
          win.webContents.openDevTools({ mode: 'detach' })
        }
      }
    })
    win.webContents.openDevTools({ mode: 'detach' })
  }

  win.once('ready-to-show', () => win.show())

  const targetURL = browserTicket
    ? new URL(`/api/local/auth/ticket?ticket=${encodeURIComponent(browserTicket)}`, contentBaseURL)
    : new URL('/', contentBaseURL)
  void win.loadURL(targetURL.toString())
  return { win, allowedOrigins }
}

module.exports = { createMainWindow }
