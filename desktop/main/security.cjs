'use strict'

const { shell } = require('electron')

function createOriginAllowlist(urls) {
  return new Set(urls.filter(Boolean).map(value => new URL(value).origin))
}

function isTrustedURL(rawURL, allowedOrigins) {
  try {
    return allowedOrigins.has(new URL(rawURL).origin)
  } catch {
    return false
  }
}

function installNavigationPolicy(win, allowedOrigins) {
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith('https://')) void shell.openExternal(url)
    return { action: 'deny' }
  })

  win.webContents.on('will-navigate', (event, url) => {
    if (isTrustedURL(url, allowedOrigins)) return
    event.preventDefault()
    if (url.startsWith('https://')) void shell.openExternal(url)
  })
}

module.exports = { createOriginAllowlist, installNavigationPolicy, isTrustedURL }
