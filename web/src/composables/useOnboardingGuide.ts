import { ref } from 'vue'

/**
 * 新手指引只存在本机，不进账号，也不保存任务内容。
 * 未写入过缓存时视为第一次进入；跳过或成功创建任务后不再自动出现。
 * 离开新增对话页时保留当前步，回来后若执行方式或工作目录已经就绪则跳过对应步。
 */
const STORAGE_KEY = 'goteams.onboarding.guide'
const STORAGE_VERSION = 1

export type OnboardingStatus = 'active' | 'dismissed' | 'completed'
export type OnboardingPhase = 'intro' | 'execution' | 'directory' | 'describe'
export type OnboardingTarget = 'execution' | 'directory' | 'input' | 'send' | null

export interface OnboardingFacts {
  hasExecutionMode: boolean
  hasWorkDirectory: boolean
}

interface PersistedOnboarding {
  version: number
  status: OnboardingStatus
  phase: OnboardingPhase
}

const STATUSES: OnboardingStatus[] = ['active', 'dismissed', 'completed']
const PHASES: OnboardingPhase[] = ['intro', 'execution', 'directory', 'describe']

const status = ref<OnboardingStatus>('active')
const phase = ref<OnboardingPhase>('intro')

function isStatus(value: unknown): value is OnboardingStatus {
  return typeof value === 'string' && STATUSES.includes(value as OnboardingStatus)
}

function isPhase(value: unknown): value is OnboardingPhase {
  return typeof value === 'string' && PHASES.includes(value as OnboardingPhase)
}

function readPersisted(): PersistedOnboarding | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<PersistedOnboarding>
    if (parsed.version !== STORAGE_VERSION || !isStatus(parsed.status) || !isPhase(parsed.phase)) {
      return null
    }
    return { version: STORAGE_VERSION, status: parsed.status, phase: parsed.phase }
  } catch {
    return null
  }
}

function persist(): void {
  try {
    const payload: PersistedOnboarding = {
      version: STORAGE_VERSION,
      status: status.value,
      phase: phase.value,
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
  } catch {
    // 隐私模式等无法写入时，本次会话仍用内存中的进度。
  }
}

function commit(nextStatus: OnboardingStatus, nextPhase: OnboardingPhase): void {
  if (status.value === nextStatus && phase.value === nextPhase) return
  status.value = nextStatus
  phase.value = nextPhase
  persist()
}

const saved = readPersisted()
if (saved) {
  status.value = saved.status
  phase.value = saved.phase
}

export function resolveOnboardingPhase(facts: OnboardingFacts): Exclude<OnboardingPhase, 'intro'> {
  if (!facts.hasExecutionMode) return 'execution'
  if (!facts.hasWorkDirectory) return 'directory'
  return 'describe'
}

export function resolveOnboardingTarget(
  currentStatus: OnboardingStatus,
  currentPhase: OnboardingPhase,
  hasPrompt: boolean,
): OnboardingTarget {
  if (currentStatus !== 'active' || currentPhase === 'intro') return null
  if (currentPhase === 'execution') return 'execution'
  if (currentPhase === 'directory') return 'directory'
  return hasPrompt ? 'send' : 'input'
}

export function useOnboardingGuide() {
  function start(facts: OnboardingFacts): void {
    if (status.value !== 'active') return
    commit('active', resolveOnboardingPhase(facts))
  }

  function reconcile(facts: OnboardingFacts): void {
    if (status.value !== 'active' || phase.value === 'intro') return
    commit('active', resolveOnboardingPhase(facts))
  }

  function skip(): void {
    commit('dismissed', phase.value)
  }

  function complete(): void {
    if (status.value !== 'active') return
    commit('completed', phase.value)
  }

  function replay(): void {
    commit('active', 'intro')
  }

  return {
    status,
    phase,
    start,
    reconcile,
    skip,
    complete,
    replay,
  }
}
