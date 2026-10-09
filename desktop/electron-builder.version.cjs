'use strict'

const { syncVersion } = require('../tools/build/sync-version.cjs')

syncVersion(true)

module.exports = {
  extends: './electron-builder.yml',
}
