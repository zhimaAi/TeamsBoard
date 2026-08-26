'use strict'

const path = require('node:path')
const { app, BrowserWindow, session } = require('electron')
const { APP_ICON_PATH } = require('./app-identity.cjs')
const { createOriginAllowlist, installNavigationPolicy } = require('./security.cjs')

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

  installNavigationPolicy(win, allowedOrigins)
  session.defaultSession.setPermissionRequestHandler((_webContents, _permission, callback) => callback(false))

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
