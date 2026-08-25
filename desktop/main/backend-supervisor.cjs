'use strict'

const { EventEmitter } = require('node:events')
const { spawn } = require('node:child_process')
const crypto = require('node:crypto')
const fs = require('node:fs')
const http = require('node:http')
const net = require('node:net')
const os = require('node:os')
const path = require('node:path')

const HEALTH_TIMEOUT_MS = 30_000
const STOP_TIMEOUT_MS = 3_000
const RUNTIME_JSON_PATH = path.join(os.homedir(), '.goteams', 'runtime', 'runtime.json')

function randomToken() {
  return crypto.randomBytes(32).toString('hex')
}

function allocateLoopbackPort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer()
    server.unref()
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      const port = typeof address === 'object' && address ? address.port : 0
      server.close(error => error ? reject(error) : resolve(port))
    })
  })
}

// 读取上次监听端口：优先复用可保持渲染器 origin 稳定（localStorage 按 origin 隔离，
// 每次换端口缓存就会"丢失"），仅在被占用时才换新端口。
function readLastBackendPort() {
  try {
    const raw = fs.readFileSync(RUNTIME_JSON_PATH, 'utf8')
    const data = JSON.parse(raw)
    const address = typeof data.address === 'string' ? data.address.trim() : ''
    const match = address.match(/:(\d+)$/)
    if (match) {
      const port = Number(match[1])
      if (Number.isInteger(port) && port > 0 && port < 65536) return port
    }
  } catch {
    // runtime.json 尚未写入（首次启动）或暂时不可读
  }
  return null
}

// 探测端口当前是否可绑定；探测后立即释放，供 sidecar 紧接着监听。
// 释放与 sidecar 绑定之间存在毫秒级窗口，loopback 上可接受。
function isPortAvailable(port) {
  return new Promise(resolve => {
    const server = net.createServer()
    server.unref()
    server.once('error', () => resolve(false))
    server.listen(port, '127.0.0.1', () => {
      server.close(() => resolve(true))
    })
  })
}

function request(url, options = {}) {
  return new Promise((resolve, reject) => {
    const req = http.request(url, options, response => {
      response.resume()
      response.on('end', () => resolve(response.statusCode || 0))
    })
    req.setTimeout(1_000, () => req.destroy(new Error('request timeout')))
    req.once('error', reject)
    req.end()
  })
}

async function waitForHealth(baseURL, child, timeout = HEALTH_TIMEOUT_MS) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`Go sidecar exited before becoming ready (code ${child.exitCode})`)
    }
    try {
      if (await request(`${baseURL}/api/local/health`) === 200) return
    } catch {
      // The process is still starting.
    }
    await new Promise(resolve => setTimeout(resolve, 200))
  }
  throw new Error(`Go sidecar did not become ready within ${timeout}ms`)
}

function resolveBackendLaunch({ isPackaged, resourcesPath, repoRoot, platform }) {
  if (!isPackaged) {
    return { command: 'go', args: ['run', './cmd/client'], cwd: repoRoot }
  }
  const executable = platform === 'win32' ? 'client.exe' : 'client'
  return {
    command: path.join(resourcesPath, 'backend', executable),
    args: [],
    cwd: path.join(resourcesPath, 'backend'),
  }
}

class BackendSupervisor extends EventEmitter {
  constructor({ isPackaged, resourcesPath, repoRoot, rendererURL }) {
    super()
    this.isPackaged = isPackaged
    this.resourcesPath = resourcesPath
    this.repoRoot = repoRoot
    this.rendererURL = rendererURL
    this.child = null
    this.baseURL = ''
    this.browserTicket = ''
    this.desktopToken = ''
    this.apiToken = ''
    this.stopping = false
  }

  async start() {
    if (this.child) return this.connectionInfo()

    // 调试固定端口优先；否则优先复用上次端口（保持渲染器 origin 稳定，localStorage 不丢），
    // 被外部占用时才回退随机端口；随机端口仅用于并发实例防碰撞。
    let port
    if (process.env.GOTEAMS_DESKTOP_BACKEND_PORT) {
      port = Number(process.env.GOTEAMS_DESKTOP_BACKEND_PORT)
    } else {
      const lastPort = readLastBackendPort()
      port = lastPort && (await isPortAvailable(lastPort))
        ? lastPort
        : await allocateLoopbackPort()
    }
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      throw new Error(`Invalid backend port: ${port}`)
    }

    this.baseURL = `http://127.0.0.1:${port}`
    this.browserTicket = randomToken()
    this.desktopToken = randomToken()
    this.apiToken = randomToken()

    const launch = resolveBackendLaunch({
      isPackaged: this.isPackaged,
      resourcesPath: this.resourcesPath,
      repoRoot: this.repoRoot,
      platform: process.platform,
    })

    this.child = spawn(launch.command, launch.args, {
      cwd: launch.cwd,
      env: {
        ...process.env,
        GOTEAMS_BROWSER_TICKET: this.browserTicket,
        GOTEAMS_DESKTOP_TOKEN: this.desktopToken,
        GOTEAMS_API_TOKEN: this.apiToken,
        GOTEAMS_LISTEN_ADDR: `127.0.0.1:${port}`,
        GOTEAMS_PARENT_PID: String(process.pid),
      },
      stdio: 'inherit',
      windowsHide: true,
    })

    this.child.once('error', error => this.emit('error', error))
    this.child.once('exit', (code, signal) => {
      this.child = null
      this.emit('exit', { code, signal, expected: this.stopping })
    })

    await waitForHealth(this.baseURL, this.child)
    return this.connectionInfo()
  }

  connectionInfo() {
    if (!this.baseURL || !this.browserTicket) {
      throw new Error('Go sidecar is not running')
    }
    return {
      baseURL: this.baseURL,
      browserTicket: this.browserTicket,
      apiToken: this.apiToken,
      rendererURL: this.rendererURL,
    }
  }

  async stop() {
    const child = this.child
    if (!child || this.stopping) return
    this.stopping = true

    try {
      await request(`${this.baseURL}/api/local/desktop/shutdown`, {
        method: 'POST',
        headers: { 'X-GoTeams-Desktop-Token': this.desktopToken },
      })
    } catch {
      // Fall through to process termination if graceful shutdown is unavailable.
    }

    await Promise.race([
      new Promise(resolve => child.once('exit', resolve)),
      new Promise(resolve => setTimeout(resolve, STOP_TIMEOUT_MS)),
    ])
    if (child.exitCode === null) {
      child.kill()
      await Promise.race([
        new Promise(resolve => child.once('exit', resolve)),
        new Promise(resolve => setTimeout(resolve, 1_000)),
      ])
    }
  }
}

module.exports = {
  BackendSupervisor,
  allocateLoopbackPort,
  resolveBackendLaunch,
  waitForHealth,
}
