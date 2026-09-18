'use strict'

const { normalizeDirectoryPath } = require('./ipc/path-validation.cjs')

const MAX_CODEX_PROMPT_LENGTH = 16 * 1024
const CODEX_THREAD_ID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

function normalizeCodexThreadID(threadId) {
  if (typeof threadId !== 'string') return ''
  const normalized = threadId.trim().toLowerCase()
  return CODEX_THREAD_ID_PATTERN.test(normalized) ? normalized : ''
}

function buildCodexNewThreadURL(directoryPath, prompt) {
  const normalizedPath = normalizeDirectoryPath(directoryPath)
  if (typeof prompt !== 'string') throw new TypeError('Codex prompt must be a string')
  const normalizedPrompt = prompt.trim()
  if (!normalizedPrompt || normalizedPrompt.length > MAX_CODEX_PROMPT_LENGTH || normalizedPrompt.includes('\0')) {
    throw new Error('Codex prompt is invalid')
  }
  const target = new URL('codex://threads/new')
  target.searchParams.set('path', normalizedPath)
  target.searchParams.set('prompt', normalizedPrompt)
  return target.toString()
}

function buildCodexThreadURL(directoryPath, prompt, threadId) {
  const normalizedThreadID = normalizeCodexThreadID(threadId)
  if (normalizedThreadID) return `codex://threads/${normalizedThreadID}`
  return buildCodexNewThreadURL(directoryPath, prompt)
}

module.exports = {
  MAX_CODEX_PROMPT_LENGTH,
  buildCodexNewThreadURL,
  buildCodexThreadURL,
  normalizeCodexThreadID,
}
