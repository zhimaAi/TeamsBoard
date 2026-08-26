'use strict'

const TASK_STATUS_STALE_MS = 30_000

function normalizeTaskCount(value) {
  if (!Number.isFinite(value)) return null
  const count = Math.floor(value)
  return count >= 0 ? count : null
}

class TrayManager {
  constructor({ Tray, Menu, iconPath, now = Date.now, onShowWindow, onHideWindow, onQuit, isWindowVisible }) {
    this.Tray = Tray
    this.Menu = Menu
    this.iconPath = iconPath
    this.now = now
    this.onShowWindow = onShowWindow
    this.onHideWindow = onHideWindow
    this.onQuit = onQuit
    this.isWindowVisible = isWindowVisible
    this.tray = null
    this.starting = true
    this.taskCount = null
    this.taskUpdatedAt = 0
  }

  create() {
    if (this.tray) return
    this.tray = new this.Tray(this.iconPath)
    this.tray.on('double-click', this.onShowWindow)
    this.refresh()
  }

  setReady() {
    this.starting = false
    this.refresh()
  }

  setTaskCount(value) {
    const count = normalizeTaskCount(value)
    if (count === null) return false
    this.taskCount = count
    this.taskUpdatedAt = this.now()
    this.refresh()
    return true
  }

  getTaskStatus() {
    if (this.taskCount === null || this.now() - this.taskUpdatedAt > TASK_STATUS_STALE_MS) {
      return { known: false, count: 0 }
    }
    return { known: true, count: this.taskCount }
  }

  refresh() {
    if (!this.tray) return
    this.tray.setToolTip(this.getTooltip())
    this.tray.setContextMenu(this.Menu.buildFromTemplate(this.getMenuTemplate()))
  }

  getTooltip() {
    if (this.starting) return 'TeamsBoard · 正在启动'
    const status = this.getTaskStatus()
    return status.known && status.count > 0 ? `TeamsBoard · ${status.count} 个任务执行中` : 'TeamsBoard'
  }

  getMenuTemplate() {
    const status = this.getTaskStatus()
    const taskLabel = this.starting
      ? '任务状态：正在启动'
      : status.known
        ? `执行中任务：${status.count}`
        : '任务状态：等待同步'
    return [
      { label: '打开 TeamsBoard', click: this.onShowWindow },
      { label: '隐藏主窗口', visible: this.isWindowVisible(), click: this.onHideWindow },
      { type: 'separator' },
      { label: taskLabel, enabled: false },
      { type: 'separator' },
      { label: '退出 TeamsBoard', click: this.onQuit },
    ]
  }

  destroy() {
    this.tray?.destroy()
    this.tray = null
  }
}

module.exports = {
  TASK_STATUS_STALE_MS,
  TrayManager,
  normalizeTaskCount,
}
