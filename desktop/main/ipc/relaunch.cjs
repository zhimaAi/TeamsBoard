'use strict'

const { app, ipcMain } = require('electron')
const { spawn } = require('node:child_process')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const channels = require('../../shared/channels.cjs')
const { isTrustedURL } = require('../security.cjs')

function devRendererURL() {
  const value = process.env.GOTEAMS_DESKTOP_RENDERER_URL || ''
  return value.startsWith('http://') || value.startsWith('https://') ? value : ''
}

// 开发模式由 concurrently -k 拉起。只 relaunch Electron 会把 Vite 一起杀掉，
// 新窗口继续打开 localhost:5173 就是白屏。等旧端口退出后，再拉起整套 dev。
function scheduleDevStackRestart() {
  const repoRoot = path.resolve(__dirname, '..', '..', '..')
  const scriptPath = path.join(os.tmpdir(), 'goteams-dev-relaunch.sh')
  const configName = process.env.GOTEAMS_CONFIG_NAME || 'dev3'
  const script = `#!/bin/bash
set -euo pipefail
for _ in $(seq 1 40); do
  if ! lsof -nP -iTCP:5173 -sTCP:LISTEN >/dev/null 2>&1 \\
    && ! lsof -nP -iTCP:53386 -sTCP:LISTEN >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done
unset ELECTRON_RUN_AS_NODE NODE_OPTIONS
export PATH="$HOME/go/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin"
export GOTEAMS_CONFIG_NAME=${JSON.stringify(configName).slice(1, -1)}
cd ${JSON.stringify(repoRoot)}
exec npm --prefix desktop run dev
`
  fs.writeFileSync(scriptPath, script, { mode: 0o700 })
  const child = spawn('/bin/bash', [scriptPath], {
    detached: true,
    stdio: 'ignore',
  })
  child.unref()
}

function registerRelaunchIPC(allowedOrigins) {
  ipcMain.handle(channels.RELAUNCH_APP, event => {
    if (!event.senderFrame || !isTrustedURL(event.senderFrame.url, allowedOrigins)) {
      throw new Error('Untrusted relaunch request')
    }
    if (devRendererURL()) {
      scheduleDevStackRestart()
    } else {
      app.relaunch()
    }
    app.quit()
    return { restarting: true }
  })

  return () => ipcMain.removeHandler(channels.RELAUNCH_APP)
}

module.exports = { registerRelaunchIPC }
