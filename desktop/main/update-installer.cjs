'use strict'

async function installOnWindows({ confirmQuit, stopBackend, spawnInstaller, quit, onFailure,
  onStart, schedule = fn => { setImmediate(fn) } }) {
  if (!await confirmQuit()) return { started: false, manual: false }
  onStart()
  schedule(async () => {
    try {
      await stopBackend()
      await spawnInstaller()
      quit()
    } catch (error) {
      onFailure(error)
    }
  })
  return { started: true, manual: false }
}

module.exports = { installOnWindows }
