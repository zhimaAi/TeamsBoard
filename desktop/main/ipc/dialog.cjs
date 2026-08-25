'use strict'

const path = require('node:path')
const { BrowserWindow, dialog, ipcMain } = require('electron')
const channels = require('../../shared/channels.cjs')
const { isTrustedURL } = require('../security.cjs')

function registerDialogIPC(allowedOrigins) {
  /** @param {Electron.IpcMainInvokeEvent} event @param {{ defaultPath?: string, multiple?: boolean }} options */
  ipcMain.handle(channels.SELECT_DIRECTORIES, async (event, options = {}) => {
    if (!event.senderFrame || !isTrustedURL(event.senderFrame.url, allowedOrigins)) {
      throw new Error('Untrusted directory picker request')
    }

    const parent = BrowserWindow.fromWebContents(event.sender)
    const defaultPath = typeof options.defaultPath === 'string' && path.isAbsolute(options.defaultPath)
      ? options.defaultPath
      : undefined
    /** @type {Electron.OpenDialogOptions['properties']} */
    const properties = ['openDirectory']
    if (options.multiple === true) properties.push('multiSelections')

    /** @type {Electron.OpenDialogOptions} */
    const dialogOptions = {
      title: '选择工作目录',
      defaultPath,
      properties,
    }
    const result = parent
      ? await dialog.showOpenDialog(parent, dialogOptions)
      : await dialog.showOpenDialog(dialogOptions)
    return result.canceled ? [] : result.filePaths
  })

  return () => ipcMain.removeHandler(channels.SELECT_DIRECTORIES)
}

module.exports = { registerDialogIPC }
