'use strict'

const { ipcMain } = require('electron')
const channels = require('../../shared/channels.cjs')
const { isTrustedURL } = require('../security.cjs')

function registerApiTokenIPC(allowedOrigins, apiToken) {
  ipcMain.handle(channels.GET_API_TOKEN, (event) => {
    if (!event.senderFrame || !isTrustedURL(event.senderFrame.url, allowedOrigins)) {
      throw new Error('Untrusted api token request')
    }
    return apiToken
  })

  return () => ipcMain.removeHandler(channels.GET_API_TOKEN)
}

module.exports = { registerApiTokenIPC }
