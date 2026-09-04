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
const LOGIN_SHELL_TIMEOUT_MS = 3_000
const LOGIN_SHELL_CLEANUP_TIMEOUT_MS = 250
const LOGIN_SHELL_MAX_BUFFER = 64 * 1024
const SHELL_PATH_MARKER = '__GOTEAMS_LOGIN_PATH__='
const SUPPORTED_LOGIN_SHELLS = new Set(['bash', 'csh', 'fish', 'ksh', 'sh', 'tcsh', 'zsh'])
const MAC_SYSTEM_PATH_DIRS = ['/usr/bin', '/bin', '/usr/sbin', '/sbin']
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

// 这些目录来自 TeamsBoard 当前支持 CLI 的已核验上游 macOS 发布渠道默认值，
// 再加 Homebrew 在 Apple Silicon/Intel 上的标准 prefix；其中 Grok CLI 是其
// 社区项目的发布渠道，并非 xAI 官方产品。TeamsBoard 只按 grok 命令名发现，
// 无法据此确认具体发行来源。npm 自定义 prefix、nvm/fnm/Volta 等版本管理器
// 不在这里猜测或扫描，由用户 shell 的有效 PATH 提供。
function macCliSearchDirs(homeDir) {
  return [
    // Codex、Claude、Cursor Agent、Copilot、Qwen 和 Pi 的官方安装器默认/回退目录。
    path.posix.join(homeDir, '.local', 'bin'),
    path.posix.join(homeDir, '.opencode', 'bin'),
    path.posix.join(homeDir, '.kimi-code', 'bin'),
    path.posix.join(homeDir, '.grok', 'bin'),
    // Pi 的可选托管安装目录；Qoder/Qoder CN 的独立安装器均公开链接到 .local/bin。
    path.posix.join(homeDir, '.pi', 'agent', 'bin'),
    '/opt/homebrew/bin',
    '/usr/local/bin',
  ]
}

function normalizePathEntries(...values) {
  const entries = values
    .flatMap(value => Array.isArray(value) ? value : String(value || '').split(':'))
    .map(entry => String(entry).trim())
    .filter(entry => entry.length > 0 && entry.length <= 4096 && path.posix.isAbsolute(entry))
  return [...new Set(entries)]
}

function parseLoginShellPath(output) {
  const lines = String(output || '').split(/\r?\n/)
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    const markerIndex = lines[index].indexOf(SHELL_PATH_MARKER)
    if (markerIndex >= 0) {
      return lines[index].slice(markerIndex + SHELL_PATH_MARKER.length).trim()
    }
  }
  return ''
}

function loginShellProbeSpecs(executable, command) {
  const name = path.basename(executable).toLowerCase()
  if (name === 'bash') {
    // Bash 的登录 shell 不会自动读取 .bashrc；分别探测交互和登录启动文件，
    // 再按交互 PATH 优先合并，以覆盖 npm/nvm 等只写入 .bashrc 的安装方式。
    return [
      { args: ['-ic', command] },
      { args: ['-lc', command] },
    ]
  }
  if (name === 'csh' || name === 'tcsh') {
    // csh 家族不接受 -l 与 -c 组合；以 “-shell” argv0 建立登录 shell，
    // 同时用 -i 读取 cshrc，从而覆盖 .login 与 .cshrc。
    return [{ args: ['-ic', command], argv0: `-${name}` }]
  }
  return [{ args: ['-ilc', command] }]
}

function accessExecutable(candidate, signal, accessImpl) {
  return new Promise(resolve => {
    let finished = false
    const finish = value => {
      if (finished) return
      finished = true
      signal?.removeEventListener('abort', onAbort)
      resolve(value)
    }
    const onAbort = () => finish(false)

    if (signal?.aborted) {
      finish(false)
      return
    }
    signal?.addEventListener('abort', onAbort, { once: true })
    Promise.resolve()
      .then(() => accessImpl(candidate, fs.constants.X_OK))
      .then(() => finish(true), () => finish(false))
  })
}

async function resolveUserShell(configuredShell, {
  signal = /** @type {AbortSignal | undefined} */ (undefined),
  accessImpl = fs.promises.access,
} = {}) {
  let accountShell = ''
  try {
    accountShell = os.userInfo().shell
  } catch {
    // 极少数受限运行环境无法读取账户信息，继续使用系统 shell 回退。
  }
  for (const candidate of [configuredShell, accountShell, '/bin/zsh', '/bin/bash']) {
    if (signal?.aborted) return ''
    if (typeof candidate !== 'string' || !path.isAbsolute(candidate)) continue
    if (!SUPPORTED_LOGIN_SHELLS.has(path.basename(candidate).toLowerCase())) continue
    if (await accessExecutable(candidate, signal, accessImpl)) return candidate
  }
  return ''
}

function terminateShellProbe(child) {
  if (!child) return
  const pid = Number(child.pid)
  // spawn({ detached: true }) 在 POSIX 上让 shell 成为独立进程组组长；
  // 负 PID 可同时清理启动文件产生且仍持有 stdout/stderr 的后代进程。
  if (process.platform !== 'win32' && Number.isInteger(pid) && pid > 0) {
    try {
      process.kill(-pid, 'SIGKILL')
      return
    } catch {
      // 子进程组可能已退出，继续尝试直接终止。
    }
  }
  try {
    child.kill('SIGKILL')
  } catch {
    // 探测进程已经退出。
  }
}

function isShellProbeGroupAlive(child) {
  const pid = Number(child?.pid)
  if (process.platform === 'win32' || !Number.isInteger(pid) || pid <= 0) return false
  try {
    process.kill(-pid, 0)
    return true
  } catch (error) {
    return error?.code !== 'ESRCH'
  }
}

async function cleanupShellProbeGroups(children, {
  timeout = LOGIN_SHELL_CLEANUP_TIMEOUT_MS,
  terminateProcessImpl = terminateShellProbe,
  processGroupAliveImpl = isShellProbeGroupAlive,
} = {}) {
  const groups = [...new Set(children)].filter(Boolean)
  for (const child of groups) terminateProcessImpl(child)

  const deadline = Date.now() + Math.max(0, timeout)
  while (groups.some(child => processGroupAliveImpl(child)) && Date.now() < deadline) {
    await new Promise(resolve => setTimeout(resolve, Math.min(10, Math.max(1, deadline - Date.now()))))
  }
  for (const child of groups) {
    child.stdout?.destroy()
    child.stderr?.destroy()
  }
}

// macOS GUI 应用没有用户终端的 PATH。-i/-l 会以当前用户权限执行其 shell
// 初始化脚本，这是明确的本机信任边界。探测命令本身固定，输出手工限制为
// 64 KiB；总截止为清理预留有界时间，并在返回前轮询确认已启动进程组退出。
function readLoginShellPath({
  environment = process.env,
  homeDir = os.homedir(),
  shell = environment.SHELL,
  timeout = LOGIN_SHELL_TIMEOUT_MS,
  cleanupTimeout = LOGIN_SHELL_CLEANUP_TIMEOUT_MS,
  maxBuffer = LOGIN_SHELL_MAX_BUFFER,
  signal = /** @type {AbortSignal | undefined} */ (undefined),
  accessImpl = fs.promises.access,
  spawnImpl = spawn,
  terminateProcessImpl = terminateShellProbe,
  processGroupAliveImpl = isShellProbeGroupAlive,
} = {}) {
  if (signal?.aborted) return Promise.resolve('')

  const command = `/usr/bin/printf '${SHELL_PATH_MARKER}'; /usr/bin/printenv PATH`
  const cleanupBudget = Math.min(
    Math.max(0, cleanupTimeout),
    Math.max(1, Math.floor(Math.max(1, timeout) / 4)),
  )
  const executionTimeout = Math.max(1, timeout - cleanupBudget)
  const probeController = new AbortController()

  return new Promise(resolve => {
    const startedChildren = new Set()
    let deadline = null
    let finishing = false
    let outputBytes = 0

    const onAbort = () => finish('')
    const finish = value => {
      if (finishing) return
      finishing = true
      if (deadline) clearTimeout(deadline)
      signal?.removeEventListener('abort', onAbort)
      probeController.abort()
      void cleanupShellProbeGroups(startedChildren, {
        timeout: cleanupBudget,
        terminateProcessImpl,
        processGroupAliveImpl,
      }).finally(() => resolve(value))
    }
    const capture = (chunk, keep, stdoutChunks) => {
      if (finishing) return
      const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(String(chunk))
      outputBytes += buffer.length
      if (outputBytes > maxBuffer) {
        finish('')
        return
      }
      if (keep) stdoutChunks.push(buffer)
    }

    signal?.addEventListener('abort', onAbort, { once: true })
    deadline = setTimeout(() => finish(''), executionTimeout)

    void resolveUserShell(shell, {
      signal: probeController.signal,
      accessImpl,
    }).then(executable => {
      if (finishing) return
      if (!executable) {
        finish('')
        return
      }

      const probeSpecs = loginShellProbeSpecs(executable, command)
      const pathOutputs = new Array(probeSpecs.length).fill('')
      let remaining = probeSpecs.length
      const complete = (index, value) => {
        if (finishing) return
        pathOutputs[index] = value
        remaining -= 1
        if (remaining === 0) finish(normalizePathEntries(pathOutputs).join(':'))
      }

      probeSpecs.forEach((spec, index) => {
        let child = null
        let settled = false
        const stdoutChunks = []
        const settle = value => {
          if (settled) return
          settled = true
          complete(index, value)
        }

        try {
          child = spawnImpl(
            executable,
            spec.args,
            {
              cwd: homeDir,
              env: { ...environment, HOME: homeDir },
              detached: true,
              stdio: ['ignore', 'pipe', 'pipe'],
              windowsHide: true,
              ...(spec.argv0 ? { argv0: spec.argv0 } : {}),
            },
          )
          startedChildren.add(child)
          child.stdout?.on('data', chunk => capture(chunk, true, stdoutChunks))
          child.stderr?.on('data', chunk => capture(chunk, false, stdoutChunks))
          child.once('error', () => settle(''))
          child.once('close', code => {
            const output = Buffer.concat(stdoutChunks).toString('utf8')
            settle(code === 0 ? parseLoginShellPath(output) : '')
          })
        } catch {
          settle('')
        }
      })
    }, () => finish(''))
  })
}

function augmentBackendPath(environment, {
  platform = process.platform,
  homeDir = os.homedir(),
  loginShellPath = '',
  cliSearchDirs = macCliSearchDirs(homeDir),
  systemPathDirs = MAC_SYSTEM_PATH_DIRS,
} = {}) {
  const result = { ...environment }
  if (platform !== 'darwin') return result

  // 用户有效 PATH 优先；Finder 继承 PATH 次之；CLI/Homebrew 回退和 macOS
  // 系统基线目录最后。系统目录不属于 CLI 安装渠道。
  result.PATH = normalizePathEntries(
    loginShellPath,
    environment.PATH,
    cliSearchDirs,
    systemPathDirs,
  ).join(':')
  return result
}

async function resolveBackendEnvironment(environment, {
  isPackaged = false,
  platform = process.platform,
  homeDir = os.homedir(),
  signal = /** @type {AbortSignal | undefined} */ (undefined),
  loginShellPathReader = readLoginShellPath,
} = {}) {
  if (!isPackaged || platform !== 'darwin') return { ...environment }

  let loginShellPath = ''
  try {
    loginShellPath = await loginShellPathReader({ environment, homeDir, signal })
  } catch {
    // shell 探测异常时只使用当前 PATH 和官方安装目录回退。
  }
  return augmentBackendPath(environment, { platform, homeDir, loginShellPath })
}

class BackendSupervisor extends EventEmitter {
  constructor({
    isPackaged,
    resourcesPath,
    repoRoot,
    rendererURL,
    spawnImpl = spawn,
    resolveBackendEnvironmentImpl = resolveBackendEnvironment,
  }) {
    super()
    this.isPackaged = isPackaged
    this.resourcesPath = resourcesPath
    this.repoRoot = repoRoot
    this.rendererURL = rendererURL
    this.spawnImpl = spawnImpl
    this.resolveBackendEnvironmentImpl = resolveBackendEnvironmentImpl
    this.child = null
    this.pathProbeController = null
    this.pathProbePromise = null
    this.baseURL = ''
    this.browserTicket = ''
    this.desktopToken = ''
    this.apiToken = ''
    this.stopping = false
  }

  async start() {
    if (this.stopping) throw new Error('Backend start cancelled')
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
    if (this.stopping) throw new Error('Backend start cancelled')

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

    const pathProbeController = new AbortController()
    this.pathProbeController = pathProbeController
    const pathProbePromise = this.resolveBackendEnvironmentImpl(process.env, {
      isPackaged: this.isPackaged,
      signal: pathProbeController.signal,
    })
    this.pathProbePromise = pathProbePromise

    let backendEnvironment
    try {
      backendEnvironment = await pathProbePromise
    } finally {
      if (this.pathProbeController === pathProbeController) this.pathProbeController = null
      if (this.pathProbePromise === pathProbePromise) this.pathProbePromise = null
    }
    if (this.stopping) throw new Error('Backend start cancelled')

    this.child = this.spawnImpl(launch.command, launch.args, {
      cwd: launch.cwd,
      env: {
        ...backendEnvironment,
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
    if (this.stopping) return
    this.stopping = true

    const pathProbePromise = this.pathProbePromise
    this.pathProbeController?.abort()
    if (pathProbePromise) {
      try {
        await pathProbePromise
      } catch {
        // 启动探测失败时仍继续退出流程。
      }
    }

    const child = this.child
    if (!child) return

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
  augmentBackendPath,
  loginShellProbeSpecs,
  macCliSearchDirs,
  normalizePathEntries,
  parseLoginShellPath,
  readLoginShellPath,
  resolveBackendEnvironment,
  resolveBackendLaunch,
  waitForHealth,
}
