'use strict'

const { ipcMain } = require('electron')
const channels = require('../../shared/channels.cjs')
const { isTrustedURL } = require('../security.cjs')
const { openInBrowser } = require('../deep-link.cjs')

const START_TIMEOUT_MS = 15000

// 浏览器登录的渲染层入口：由主进程代渲染层向 Go sidecar 申请登录会话并打开系统浏览器。
// 渲染层不直接传 URL，避免把任意地址交给 shell.openExternal。
function registerBrowserLoginIPC(allowedOrigins, getConnection, fetchImpl = fetch) {
  ipcMain.handle(channels.START_BROWSER_LOGIN, async (event, payload) => {
    if (!event.senderFrame || !isTrustedURL(event.senderFrame.url, allowedOrigins)) {
      throw new Error('Untrusted browser login request')
    }
    const connection = getConnection()
    if (!connection) throw new Error('Go sidecar is not running')

    const body = {}
    if (payload && typeof payload === 'object') {
      if (payload.serverType === 'custom' || payload.serverType === 'official') {
        body.server_type = payload.serverType
      }
      if (typeof payload.serverUrl === 'string' && payload.serverUrl) {
        body.server_url = payload.serverUrl
      }
    }

    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), START_TIMEOUT_MS)
    let result = null
    try {
      const response = await fetchImpl(new URL('/api/local/auth/browser-login/start', connection.baseURL), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-GoTeams-Api-Token': connection.apiToken,
        },
        body: JSON.stringify(body),
        signal: controller.signal,
      })
      const text = await response.text()
      let parsed = null
      try {
        parsed = text ? JSON.parse(text) : null
      } catch {
        parsed = null
      }
      if (!response.ok) {
        throw new Error((parsed && (parsed.error || parsed.message)) || `HTTP ${response.status}`)
      }
      result = parsed
    } finally {
      clearTimeout(timer)
    }

    if (!result || !result.browser_url || !result.state) {
      throw new Error('Browser login start response is invalid')
    }
    await openInBrowser(result.browser_url)
    return {
      state: result.state,
      browserUrl: result.browser_url,
      expiresIn: Number.isFinite(result.expires_in) ? result.expires_in : 0,
    }
  })

  return () => ipcMain.removeHandler(channels.START_BROWSER_LOGIN)
}

// 把深链接回注结果推给渲染层，让界面即时刷新，不必等下一次轮询。
function notifyBrowserLoginResult(getMainWindow, payload) {
  const win = getMainWindow()
  if (!win || win.isDestroyed()) return
  const send = () => {
    if (!win.isDestroyed()) win.webContents.send(channels.BROWSER_LOGIN_RESULT, payload)
  }
  if (win.webContents.isLoadingMainFrame()) {
    win.webContents.once('did-finish-load', send)
    return
  }
  send()
}

module.exports = { registerBrowserLoginIPC, notifyBrowserLoginResult }
