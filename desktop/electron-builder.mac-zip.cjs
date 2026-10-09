'use strict'

module.exports = {
  extends: './electron-builder.version.cjs',
  mac: {
    target: ['zip'],
  },
}
