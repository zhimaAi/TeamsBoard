'use strict'

const { BrowserWindow, Notification, ipcMain } = require('electron')
const channels = require('../../shared/channels.cjs')
const { isTrustedURL } = require('../security.cjs')
const { APP_ICON_PATH } = require('../app-identity.cjs')
const { t } = require('../desktop-i18n.cjs')

const MAX_TITLE_LENGTH = 120
const MAX_BODY_LENGTH = 512
const MAX_UUID_LENGTH = 128
const MAX_ACTIVE_NOTIFICATIONS = 100

function notificationOptions(payload, platform) {
  const common = { title: payload.title, body: payload.body, silent: false }
  if (platform === 'darwin') {
    return { ...common, groupId: payload.taskUuid, actions: [{ type: 'button', text: t('notification.action.openTask') }] }
  }
  if (platform === 'win32') {
    return { ...common, icon: APP_ICON_PATH, groupId: payload.taskUuid, timeoutType: 'default' }
  }
  return common
}

function notificationFailureMessage(platform) {
  if (platform === 'darwin') return t('notification.failure.darwin')
  if (platform === 'win32') return t('notification.failure.win32')
  return t('notification.failure.generic')
}

function normalizeText(value, maxLength) {
  if (typeof value !== 'string') return ''
  return value.trim().slice(0, maxLength)
}

function normalizeTaskNotificationPayload(payload) {
  const title = normalizeText(payload?.title, MAX_TITLE_LENGTH)
  const body = normalizeText(payload?.body, MAX_BODY_LENGTH)
  const taskUuid = normalizeText(payload?.taskUuid, MAX_UUID_LENGTH)
  const stepUuid = normalizeText(payload?.stepUuid, MAX_UUID_LENGTH)

  if (!title) throw new Error('Notification title is required')
  if (!body) throw new Error('Notification body is required')
  if (!taskUuid) throw new Error('Notification taskUuid is required')

  return stepUuid ? { title, body, taskUuid, stepUuid } : { title, body, taskUuid }
}

function assertTrusted(event, allowedOrigins) {
  if (!event.senderFrame || !isTrustedURL(event.senderFrame.url, allowedOrigins)) {
    throw new Error('Untrusted task notification request')
  }
}

function focusWindow(win) {
  if (!win || win.isDestroyed()) return
  if (win.isMinimized()) win.restore()
  win.show()
  win.focus()
}

function sendOpenTaskConversation(win, payload) {
  if (!win || win.isDestroyed()) return
  const webContents = win.webContents
  if (!webContents || webContents.isDestroyed()) return

  const send = () => {
    if (!win.isDestroyed() && !webContents.isDestroyed()) {
      const target = payload.stepUuid
        ? { taskUuid: payload.taskUuid, stepUuid: payload.stepUuid }
        : { taskUuid: payload.taskUuid }
      webContents.send(channels.OPEN_TASK_CONVERSATION, target)
    }
  }

  const isLoading = typeof webContents.isLoadingMainFrame === 'function'
    ? webContents.isLoadingMainFrame()
    : webContents.isLoading()
  if (isLoading) {
    webContents.once('did-finish-load', send)
  } else {
    send()
  }
}

/**
 * @param {Set<string>} allowedOrigins
 * @param {{
 *   NotificationClass?: any,
 *   ipcMainObject?: any,
 *   getMainWindow?: () => any,
 *   showOrCreateMainWindow?: () => void,
 *   platform?: string,
 *   showTimeoutMs?: number,
 *   logger?: Pick<Console, 'info' | 'warn'>,
 * }} [options]
 */
function registerTaskNotificationIPC(allowedOrigins, options = {}) {
  const {
    NotificationClass = Notification,
    ipcMainObject = ipcMain,
    getMainWindow = () => BrowserWindow.getAllWindows()[0],
    showOrCreateMainWindow,
    platform = process.platform,
    showTimeoutMs = 15_000,
    logger = console,
  } = options
  // macOS 会在 Notification 对象被回收时撤下通知；Windows 横幅超时后
  // 仍可能从通知中心点击，因此不能在 show 返回或横幅超时时释放对象。
  const activeNotifications = new Map()
  function deliverNotification(payload) {
    if (!NotificationClass.isSupported()) return { shown: false, reason: 'unsupported' }

    const notification = new NotificationClass(notificationOptions(payload, platform))
    const openTask = () => {
      if (typeof showOrCreateMainWindow === 'function') {
        showOrCreateMainWindow()
      } else {
        focusWindow(getMainWindow())
      }
      const win = getMainWindow()
      focusWindow(win)
      sendOpenTaskConversation(win, payload)
    }
    notification.on('click', openTask)
    if (platform === 'darwin') notification.on('action', openTask)

    return new Promise(resolve => {
      let settled = false
      const settle = result => {
        if (settled) return
        settled = true
        clearTimeout(timer)
        resolve(result)
      }
      const dispose = () => {
        settle({ shown: false, reason: 'closed' })
        activeNotifications.delete(notification)
        notification.close()
        notification.removeAllListeners()
      }
      const timer = setTimeout(() => {
        logger.warn('[TaskNotification] native delivery unconfirmed', { platform })
        settle({ shown: false, reason: 'unconfirmed', message: notificationFailureMessage(platform) })
      }, showTimeoutMs)
      activeNotifications.set(notification, dispose)
      if (activeNotifications.size > MAX_ACTIVE_NOTIFICATIONS) {
        activeNotifications.values().next().value()
      }
      notification.on('show', () => {
        logger.info('[TaskNotification] native show', { platform })
        settle({ shown: true })
      })
      notification.on('failed', (_event, error) => {
        logger.warn('[TaskNotification] native failed', { platform, error: String(error).slice(0, 256) })
        settle({ shown: false, reason: 'failed', message: notificationFailureMessage(platform) })
        dispose()
      })
      notification.on('close', () => {
        // Windows 的 timedOut 只是收起横幅，不代表通知中心已移除。
        if (platform !== 'win32') {
          activeNotifications.delete(notification)
          settle({ shown: false, reason: 'closed' })
        }
      })
      try {
        notification.show()
      } catch (error) {
        notification.emit('failed', undefined, error)
      }
    })
  }

  ipcMainObject.handle(channels.SHOW_TASK_NOTIFICATION, (event, rawPayload) => {
    assertTrusted(event, allowedOrigins)
    return deliverNotification(normalizeTaskNotificationPayload(rawPayload))
  })

  return () => {
    ipcMainObject.removeHandler(channels.SHOW_TASK_NOTIFICATION)
    for (const dispose of activeNotifications.values()) dispose()
  }
}

module.exports = {
  MAX_BODY_LENGTH,
  MAX_TITLE_LENGTH,
  MAX_UUID_LENGTH,
  notificationOptions,
  notificationFailureMessage,
  normalizeTaskNotificationPayload,
  registerTaskNotificationIPC,
  sendOpenTaskConversation,
}
