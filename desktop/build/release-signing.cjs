'use strict'

const fs = require('node:fs')
const path = require('node:path')

const DEFAULT_CONFIG_PATH = path.resolve(__dirname, '..', '..', 'signing.config.json')
const SHA1_PATTERN = /^[A-F0-9]{40}$/

function readSigningConfig(
  configPath = process.env.GOTEAMS_SIGNING_CONFIG || DEFAULT_CONFIG_PATH,
) {
  let raw
  try {
    raw = fs.readFileSync(configPath, 'utf8')
  } catch (error) {
    if (error && error.code === 'ENOENT') {
      throw new Error(
        `缺少发布签名配置：${configPath}\n` +
          '请复制 signing.config.example.json 为 signing.config.json，并填写 signtoolPath。',
      )
    }
    throw error
  }

  try {
    return JSON.parse(raw)
  } catch (error) {
    throw new Error(`发布签名配置不是合法 JSON：${configPath}\n${error.message}`)
  }
}

function loadReleaseSigningConfig(
  configPath = process.env.GOTEAMS_SIGNING_CONFIG || DEFAULT_CONFIG_PATH,
) {
  const config = readSigningConfig(configPath)
  const configuredPath = typeof config.signtoolPath === 'string'
    ? config.signtoolPath.trim()
    : ''

  if (!configuredPath) {
    throw new Error('signtoolPath 不能为空；请填写微软 Windows SDK signtool.exe 的完整路径。')
  }

  const signtoolPath = path.resolve(configuredPath)
  if (!fs.existsSync(signtoolPath)) {
    throw new Error(`找不到微软 Signtool：${signtoolPath}`)
  }

  return Object.freeze({ signtoolPath })
}

function normalizeCertificateSha1(value) {
  const normalized = String(value || '')
    .replace(/[\s:\u200e\u200f\u202a-\u202e\u2066-\u2069]/g, '')
    .toUpperCase()

  if (!SHA1_PATTERN.test(normalized)) {
    throw new Error('证书指纹必须是 40 位 SHA-1 十六进制字符串（允许包含空格）。')
  }
  return normalized
}

function buildSigntoolArguments(certificateSha1, file, hash = 'sha256') {
  return [
    'sign',
    '/v',
    '/fd', hash.toLowerCase(),
    '/sha1', normalizeCertificateSha1(certificateSha1),
    '/tr', 'http://timestamp.sectigo.com',
    '/td', 'sha256',
    file,
  ]
}

module.exports = {
  DEFAULT_CONFIG_PATH,
  buildSigntoolArguments,
  loadReleaseSigningConfig,
  normalizeCertificateSha1,
}
