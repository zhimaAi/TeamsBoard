'use strict'

const fs = require('node:fs')
const path = require('node:path')

const CLOSE_BEHAVIORS = Object.freeze(['hide', 'quit'])
const DEFAULT_PREFERENCES = Object.freeze({
  closeBehavior: 'hide',
  closeTipShown: false,
})

function normalizePreferences(value) {
  const source = value && typeof value === 'object' ? value : {}
  return {
    closeBehavior: CLOSE_BEHAVIORS.includes(source.closeBehavior) ? source.closeBehavior : 'hide',
    closeTipShown: source.closeTipShown === true,
  }
}

class DesktopPreferences {
  constructor(userDataPath) {
    this.filePath = path.join(userDataPath, 'desktop-preferences.json')
    this.value = null
  }

  get() {
    if (this.value) return { ...this.value }

    try {
      this.value = normalizePreferences(JSON.parse(fs.readFileSync(this.filePath, 'utf8')))
    } catch {
      this.value = { ...DEFAULT_PREFERENCES }
    }
    return { ...this.value }
  }

  setCloseBehavior(closeBehavior) {
    if (!CLOSE_BEHAVIORS.includes(closeBehavior)) {
      throw new Error('Invalid desktop close behavior')
    }
    this.value = {
      ...this.get(),
      closeBehavior,
      closeTipShown: true,
    }
    this.write()
    return this.get()
  }

  write() {
    const directory = path.dirname(this.filePath)
    const temporaryPath = `${this.filePath}.tmp`
    fs.mkdirSync(directory, { recursive: true })
    fs.writeFileSync(temporaryPath, JSON.stringify(this.value), 'utf8')
    fs.renameSync(temporaryPath, this.filePath)
  }
}

module.exports = {
  CLOSE_BEHAVIORS,
  DEFAULT_PREFERENCES,
  DesktopPreferences,
  normalizePreferences,
}
