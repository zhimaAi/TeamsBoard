'use strict'

const fs = require('node:fs')
const fsp = require('node:fs/promises')
const path = require('node:path')
const https = require('node:https')
const { createHash } = require('node:crypto')
const { EventEmitter } = require('node:events')
const { Transform } = require('node:stream')
const { pipeline } = require('node:stream/promises')
const { t } = require('./desktop-i18n.cjs')

const RELEASE_URL = 'https://api.github.com/repos/zhimaAi/TeamsBoard/releases/latest'
const CACHE_TTL_MS = 30 * 60 * 1000
const MAX_RELEASE_BYTES = 2 * 1024 * 1024
const ASSET_NAMES = Object.freeze({
  'win32:x64': 'TeamsBoard-windows-x64.exe',
  'win32:arm64': 'TeamsBoard-windows-arm64.exe',
  'darwin:x64': 'TeamsBoard-macos-x64.dmg',
  'darwin:arm64': 'TeamsBoard-macos-arm64.dmg',
})

function parseVersion(value) {
  if (typeof value !== 'string' || value.length > 50) return null
  const match = /^v?(\d+)\.(\d+)\.(\d+)$/.exec(value)
  if (!match) return null
  const parts = match.slice(1).map(Number)
  return parts.every(Number.isSafeInteger) ? parts : null
}

function compareVersions(left, right) {
  const a = parseVersion(left)
  const b = parseVersion(right)
  if (!a || !b) throw new Error('Invalid release version')
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] > b[i] ? 1 : -1
  }
  return 0
}

function localDate(now) {
  const date = new Date(now)
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

function expectedAssetName(platform, arch) {
  return ASSET_NAMES[`${platform}:${arch}`] || ''
}

function statusSuffix(error) {
  const match = /HTTP (\d{3})/.exec(error instanceof Error ? error.message : String(error))
  return match ? ` (HTTP ${match[1]})` : ''
}

function errorDetail(error) {
  const message = error instanceof Error ? error.message : String(error)
  return message.replace(/[\r\n\t]+/g, ' ').trim().slice(0, 500)
}

function validAssetURL(value) {
  try {
    const url = new URL(value)
    return url.protocol === 'https:' && url.hostname === 'github.com' &&
      url.pathname.toLowerCase().startsWith('/zhimaai/teamsboard/releases/download/')
  } catch {
    return false
  }
}

function assetURLMatchesName(url, name) {
  try { return decodeURIComponent(new URL(url).pathname.split('/').at(-1)) === name } catch { return false }
}

function validRedirectURL(value) {
  const url = new URL(value)
  return url.protocol === 'https:' &&
    (url.hostname === 'github.com' || url.hostname === 'api.github.com' ||
      url.hostname === 'githubusercontent.com' || url.hostname.endsWith('.githubusercontent.com'))
}

function normalizeRelease(raw, platform, arch) {
  if (!raw || raw.draft || raw.prerelease || !parseVersion(raw.tag_name)) {
    throw new Error('GitHub Release 版本无效')
  }
  const name = expectedAssetName(platform, arch)
  const asset = raw.assets?.find(item => item.name === name && item.state === 'uploaded')
  return {
    version: raw.tag_name.replace(/^v/, ''),
    publishedAt: typeof raw.published_at === 'string' ? raw.published_at : '',
    notes: typeof raw.body === 'string' ? raw.body.slice(0, 100_000) : '',
    asset: asset && validAssetURL(asset.browser_download_url) &&
      assetURLMatchesName(asset.browser_download_url, name) && Number.isSafeInteger(asset.size) && asset.size > 0
      ? {
          name,
          size: asset.size,
          url: asset.browser_download_url,
          digest: /^sha256:[a-f0-9]{64}$/i.test(asset.digest || '') ? asset.digest.toLowerCase() : '',
        }
      : null,
  }
}

function requestResponse(url, headers, signal, redirects = 0) {
  if (!validRedirectURL(url) || redirects > 5) return Promise.reject(new Error('不安全的下载地址'))
  return new Promise((resolve, reject) => {
    const req = https.get(url, { headers, signal }, response => {
      if ([301, 302, 303, 307, 308].includes(response.statusCode)) {
        const next = new URL(response.headers.location || '', url).toString()
        response.resume()
        requestResponse(next, headers, signal, redirects + 1).then(resolve, reject)
        return
      }
      resolve(response)
    })
    req.setTimeout(30_000, () => req.destroy(new Error('请求超时')))
    req.once('error', reject)
  })
}

async function readLimited(response, limit) {
  const chunks = []
  let size = 0
  for await (const chunk of response) {
    size += chunk.length
    if (size > limit) throw new Error('GitHub Release 响应过大')
    chunks.push(chunk)
  }
  return Buffer.concat(chunks).toString('utf8')
}

async function verifyFile(filePath, asset) {
  try {
    if ((await fsp.stat(filePath)).size !== asset.size) return false
    if (!asset.digest) return true
    const hash = createHash('sha256')
    for await (const chunk of fs.createReadStream(filePath)) hash.update(chunk)
    return `sha256:${hash.digest('hex')}` === asset.digest
  } catch {
    return false
  }
}

class UpdateManager extends EventEmitter {
  constructor({ userDataPath, currentVersion, platform = process.platform, arch = process.arch,
    isPackaged, now = Date.now, getResponse = requestResponse }) {
    super()
    this.currentVersion = currentVersion
    this.platform = platform
    this.arch = arch
    this.isPackaged = isPackaged
    this.now = now
    this.getResponse = getResponse
    this.directory = path.join(userDataPath, 'updates')
    this.cachePath = path.join(this.directory, 'release-cache.json')
    this.cache = { release: null, etag: '', fetchedAt: 0, deferredDate: '' }
    try {
      const saved = JSON.parse(fs.readFileSync(this.cachePath, 'utf8'))
      if (saved && typeof saved === 'object') {
        this.cache = {
          release: saved.release || null,
          etag: typeof saved.etag === 'string' && saved.etag.length < 200 ? saved.etag : '',
          fetchedAt: Number.isFinite(saved.fetchedAt) ? saved.fetchedAt : 0,
          deferredDate: typeof saved.deferredDate === 'string' ? saved.deferredDate : '',
        }
      }
    } catch { /* 首次运行或损坏缓存按空缓存处理。 */ }
    this.release = null
    this.status = 'idle'
    this.progress = null
    this.error = ''
    this.checkPromise = null
    this.checkForced = false
    this.downloadPromise = null
    this.downloadController = null
    this.cacheWrite = Promise.resolve()
  }

  state() {
    return {
      currentVersion: this.currentVersion,
      status: this.status,
      release: this.release ? {
        version: this.release.version,
        publishedAt: this.release.publishedAt,
        notes: this.release.notes,
        asset: this.release.asset ? { name: this.release.asset.name, size: this.release.asset.size } : null,
      } : null,
      progress: this.progress,
      error: this.error,
      installSupported: this.isPackaged,
      platform: this.platform,
    }
  }

  emitState() { this.emit('state', this.state()) }

  async saveCache() {
    const snapshot = JSON.stringify(this.cache)
    this.cacheWrite = this.cacheWrite.catch(() => {}).then(async () => {
      await fsp.mkdir(this.directory, { recursive: true })
      const temp = `${this.cachePath}.tmp`
      await fsp.writeFile(temp, snapshot, 'utf8')
      await fsp.rename(temp, this.cachePath)
    })
    return this.cacheWrite
  }

  installerPath() {
    if (!this.release?.asset) throw new Error('当前系统没有可用安装包')
    return path.join(this.directory, this.release.version, this.release.asset.name)
  }

  async fetchRelease(force) {
    let cachedRelease = null
    if (this.cache.release) {
      try { cachedRelease = normalizeRelease(this.cache.release, this.platform, this.arch) }
      catch { this.cache.release = null; this.cache.etag = '' }
    }
    const age = this.now() - this.cache.fetchedAt
    if (!force && cachedRelease && age >= 0 && age < CACHE_TTL_MS) return cachedRelease
    const headers = { Accept: 'application/vnd.github+json', 'User-Agent': 'TeamsBoard-Updater',
      'X-GitHub-Api-Version': '2022-11-28' }
    if (this.cache.etag) headers['If-None-Match'] = this.cache.etag
    const response = await this.getResponse(RELEASE_URL, headers)
    if (response.statusCode === 304) {
      response.resume()
      if (!cachedRelease) throw new Error('更新缓存无效，请重试')
      this.cache.fetchedAt = this.now()
      await this.saveCache()
      return cachedRelease
    }
    if (response.statusCode !== 200) {
      let githubMessage = ''
      try {
        const body = JSON.parse(await readLimited(response, 8 * 1024))
        if (typeof body.message === 'string') githubMessage = errorDetail(body.message)
      } catch { /* 非 JSON 或响应过大时仍保留 HTTP 状态。 */ }
      throw new Error(`GitHub 返回 HTTP ${response.statusCode}${githubMessage ? `：${githubMessage}` : ''}`)
    }
    const raw = JSON.parse(await readLimited(response, MAX_RELEASE_BYTES))
    const release = normalizeRelease(raw, this.platform, this.arch)
    this.cache.release = raw
    this.cache.etag = typeof response.headers.etag === 'string' ? response.headers.etag : ''
    this.cache.fetchedAt = this.now()
    await this.saveCache()
    return release
  }

  async check(manual = false) {
    if (!manual && !this.isPackaged) return { ...this.state(), showPrompt: false }
    if (this.downloadPromise) return { ...this.state(), showPrompt: manual }
    if (this.checkPromise) {
      await this.checkPromise
      if (!manual || this.checkForced) return this.checkResult(manual)
    }
    this.checkForced = manual
    this.checkPromise = this.performCheck(manual)
    try { await this.checkPromise } finally { this.checkPromise = null }
    return this.checkResult(manual)
  }

  checkResult(manual) {
    return { ...this.state(), showPrompt: this.status === 'available' || this.status === 'downloaded'
      ? (manual || this.cache.deferredDate !== localDate(this.now())) : false }
  }

  async performCheck(force) {
    this.status = 'checking'
    this.error = ''
    this.emitState()
    try {
      const release = await this.fetchRelease(force)
      this.release = release
      if (compareVersions(release.version, this.currentVersion) <= 0) {
        this.status = 'current'
      } else if (!release.asset) {
        this.status = 'error'
        this.error = t('update.assetMissing')
      } else {
        this.status = await verifyFile(this.installerPath(), release.asset) ? 'downloaded' : 'available'
      }
    } catch (error) {
      this.release = null
      this.status = 'error'
      const detail = errorDetail(error)
      this.error = detail ? `${t('update.checkFailed')}：${detail}` : t('update.checkFailed')
    }
    this.emitState()
  }

  async defer() {
    this.cache.deferredDate = localDate(this.now())
    await this.saveCache()
  }

  async download() {
    if (this.downloadPromise) return this.downloadPromise
    this.downloadController = new AbortController()
    this.downloadPromise = this.startDownload(this.downloadController.signal)
    try { return await this.downloadPromise } finally {
      this.downloadPromise = null
      this.downloadController = null
    }
  }

  async startDownload(signal) {
    if (!this.isPackaged || !this.release?.asset || !['available', 'error', 'downloaded'].includes(this.status)) {
      throw new Error(t('update.unavailable'))
    }
    if (await verifyFile(this.installerPath(), this.release.asset)) {
      this.status = 'downloaded'
      this.emitState()
      return this.state()
    }
    return this.performDownload(signal)
  }

  async cancelDownload() {
    this.downloadController?.abort()
    if (this.downloadPromise) await this.downloadPromise.catch(() => {})
  }

  async performDownload(signal) {
    const asset = this.release.asset
    const target = this.installerPath()
    const partial = `${target}.part`
    this.status = 'downloading'
    this.error = ''
    this.progress = { received: 0, total: asset.size, bytesPerSecond: 0 }
    this.emitState()
    try {
      await fsp.mkdir(path.dirname(target), { recursive: true })
      await fsp.rm(partial, { force: true })
      if (signal.aborted) throw new Error('下载已取消')
      const response = await this.getResponse(asset.url, { 'User-Agent': 'TeamsBoard-Updater' }, signal)
      if (response.statusCode !== 200) {
        response.resume()
        throw new Error(`下载安装包失败：HTTP ${response.statusCode}`)
      }
      const output = fs.createWriteStream(partial, { flags: 'wx' })
      const hash = createHash('sha256')
      let received = 0
      let lastAt = this.now()
      let lastBytes = 0
      const meter = new Transform({
        transform: (chunk, _encoding, callback) => {
          received += chunk.length
          if (received > asset.size) { callback(new Error('安装包大小不匹配')); return }
          hash.update(chunk)
          const elapsed = this.now() - lastAt
          if (elapsed >= 250) {
            this.progress = { received, total: asset.size,
              bytesPerSecond: Math.max(0, Math.round((received - lastBytes) * 1000 / elapsed)) }
            this.emitState()
            lastAt = this.now()
            lastBytes = received
          }
          callback(null, chunk)
        },
      })
      await pipeline(response, meter, output, { signal })
      if (signal.aborted) throw new Error('下载已取消')
      if (received !== asset.size || (asset.digest && `sha256:${hash.digest('hex')}` !== asset.digest)) {
        throw new Error('安装包校验失败')
      }
      await fsp.rm(target, { force: true })
      await fsp.rename(partial, target)
      this.progress = { received, total: asset.size, bytesPerSecond: 0 }
      this.status = 'downloaded'
    } catch (error) {
      await fsp.rm(partial, { force: true }).catch(() => {})
      this.progress = null
      this.status = signal.aborted ? 'available' : 'error'
      const invalid = /安装包大小不匹配|安装包校验失败/.test(error instanceof Error ? error.message : String(error))
      this.error = signal.aborted ? '' : invalid ? t('update.packageInvalid') : t('update.downloadFailed') + statusSuffix(error)
    }
    this.emitState()
    return this.state()
  }

  async verifiedInstallerPath() {
    if (!this.isPackaged || this.status !== 'downloaded' || !this.release?.asset ||
        !await verifyFile(this.installerPath(), this.release.asset)) {
      throw new Error(t('update.installInvalid'))
    }
    return this.installerPath()
  }
}

module.exports = { UpdateManager, compareVersions, expectedAssetName, normalizeRelease, localDate,
  verifyFile, CACHE_TTL_MS }
