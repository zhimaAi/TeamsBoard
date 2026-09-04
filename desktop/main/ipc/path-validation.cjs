'use strict'

const path = require('node:path')

const MAX_DIRECTORY_PATH_LENGTH = 4096

function normalizeDirectoryPath(value) {
  if (typeof value !== 'string') {
    throw new TypeError('Directory path must be a string')
  }
  const trimmed = value.trim()
  if (!trimmed || trimmed.length > MAX_DIRECTORY_PATH_LENGTH || trimmed.includes('\0')) {
    throw new Error('Directory path is invalid')
  }
  if (!path.isAbsolute(trimmed)) {
    throw new Error('Directory path must be absolute')
  }
  return path.normalize(trimmed)
}

module.exports = { MAX_DIRECTORY_PATH_LENGTH, normalizeDirectoryPath }
