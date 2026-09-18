'use strict'

const fs = require('node:fs/promises')
const { ipcMain, shell } = require('electron')
const channels = require('../../shared/channels.cjs')
const { buildCodexThreadURL, normalizeCodexThreadID } = require('../codex-url.cjs')
const { t } = require('../desktop-i18n.cjs')
const { isTrustedURL } = require('../security.cjs')
const { normalizeDirectoryPath } = require('./path-validation.cjs')

function registerCodexIPC(allowedOrigins) {
  ipcMain.handle(channels.OPEN_CODEX_THREAD, async (event, payload = {}) => {
    if (!event.senderFrame || !isTrustedURL(event.senderFrame.url, allowedOrigins)) {
      throw new Error('Untrusted Codex request')
    }
    try {
      const threadId = normalizeCodexThreadID(payload.threadId)
      if (threadId) {
        await shell.openExternal(buildCodexThreadURL('', '', threadId))
        return { opened: true }
      }
      const requestedPath = normalizeDirectoryPath(payload.directoryPath)
      const realPath = await fs.realpath(requestedPath)
      const info = await fs.stat(realPath)
      if (!info.isDirectory()) throw new Error('not a directory')
      const targetURL = buildCodexThreadURL(realPath, payload.prompt)
      await shell.openExternal(targetURL)
      return { opened: true }
    } catch {
      throw new Error(t('error.codex.openFailed'))
    }
  })

  return () => ipcMain.removeHandler(channels.OPEN_CODEX_THREAD)
}

module.exports = { registerCodexIPC }
