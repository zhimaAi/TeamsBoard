'use strict'

const { ipcMain } = require('electron')
const channels = require('../../shared/channels.cjs')
const { CLOSE_BEHAVIORS } = require('../desktop-preferences.cjs')
const { SUPPORTED_LOCALES, setLocale } = require('../desktop-i18n.cjs')
const { normalizeTaskCount } = require('../tray-manager.cjs')
const { isTrustedURL } = require('../security.cjs')

function assertTrusted(event, allowedOrigins) {
  if (!event.senderFrame || !isTrustedURL(event.senderFrame.url, allowedOrigins)) {
    throw new Error('Untrusted tray request')
  }
}

function registerTrayIPC(allowedOrigins, preferences, trayManager) {
  ipcMain.handle(channels.GET_CLOSE_BEHAVIOR, event => {
    assertTrusted(event, allowedOrigins)
    return { closeBehavior: preferences.get().closeBehavior }
  })

  ipcMain.handle(channels.SET_CLOSE_BEHAVIOR, (event, closeBehavior) => {
    assertTrusted(event, allowedOrigins)
    if (!CLOSE_BEHAVIORS.includes(closeBehavior)) {
      throw new Error('Invalid desktop close behavior')
    }
    return { closeBehavior: preferences.setCloseBehavior(closeBehavior).closeBehavior }
  })

  ipcMain.handle(channels.SET_LOCALE, (event, locale) => {
    assertTrusted(event, allowedOrigins)
    if (!SUPPORTED_LOCALES.includes(locale)) {
      throw new Error('Invalid desktop locale')
    }
    // 先落盘再切当前语言：写盘失败时不改变已生效的托盘语言，避免重启后与当前界面不一致。
    const persisted = preferences.setLocale(locale).locale
    setLocale(persisted)
    trayManager.refresh()
    return { locale: persisted }
  })

  const handleTrayStatus = (event, status) => {
    if (!event.senderFrame || !isTrustedURL(event.senderFrame.url, allowedOrigins)) return
    const count = normalizeTaskCount(status?.runningTaskCount)
    if (count !== null) trayManager.setTaskCount(count)
  }
  ipcMain.on(channels.TRAY_STATUS, handleTrayStatus)

  return () => {
    ipcMain.removeHandler(channels.GET_CLOSE_BEHAVIOR)
    ipcMain.removeHandler(channels.SET_CLOSE_BEHAVIOR)
    ipcMain.removeHandler(channels.SET_LOCALE)
    ipcMain.removeListener(channels.TRAY_STATUS, handleTrayStatus)
  }
}

module.exports = { registerTrayIPC }
