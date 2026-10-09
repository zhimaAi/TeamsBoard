'use strict'

const { ipcMain } = require('electron')
const channels = require('../../shared/channels.cjs')
const { isTrustedURL } = require('../security.cjs')

function registerUpdateIPC(allowedOrigins, manager, install) {
  const trusted = event => {
    if (!event.senderFrame || !isTrustedURL(event.senderFrame.url, allowedOrigins)) {
      throw new Error('Untrusted update request')
    }
  }
  const handlers = {
    [channels.UPDATE_STATE]: event => { trusted(event); return manager.state() },
    [channels.UPDATE_CHECK]: (event, manual) => {
      trusted(event)
      if (typeof manual !== 'boolean') throw new Error('Invalid update check mode')
      return manager.check(manual)
    },
    [channels.UPDATE_DEFER]: event => { trusted(event); return manager.defer() },
    [channels.UPDATE_DOWNLOAD]: event => { trusted(event); return manager.download() },
    [channels.UPDATE_CANCEL]: event => { trusted(event); return manager.cancelDownload() },
    [channels.UPDATE_INSTALL]: event => { trusted(event); return install() },
  }
  for (const [channel, handler] of Object.entries(handlers)) ipcMain.handle(channel, handler)
  const onState = state => {
    for (const win of require('electron').BrowserWindow.getAllWindows()) {
      if (!win.isDestroyed()) win.webContents.send(channels.UPDATE_CHANGED, state)
    }
  }
  manager.on('state', onState)
  return () => {
    for (const channel of Object.keys(handlers)) ipcMain.removeHandler(channel)
    manager.off('state', onState)
  }
}

module.exports = { registerUpdateIPC }
