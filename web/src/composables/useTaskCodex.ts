import apiClient from '@/api/client'
import { isDesktopRuntime, openCodexThread, openVibeCli } from '@/composables/useDesktop'
import { copyText } from '@/utils/clipboard'

export interface VibeToolCapability {
  available: boolean
  installed?: boolean
  reason_code?: string
  message?: string
  launch?: 'app' | 'terminal'
  exec_path?: string
}

export interface TaskCodexContext {
  task_uuid: string
  work_dir: string
  prompt: string
  thread_id?: string
  tool?: string
  launch?: 'app' | 'terminal'
  exec_path?: string
}

export interface CodexCapability {
  available: boolean
  reason_code?: string
  message?: string
  tools?: Record<string, VibeToolCapability>
}

export interface CodexOpenResult {
  opened: boolean
  copied: boolean
  error?: Error
}

export async function loadCodexCapability(): Promise<CodexCapability> {
  return apiClient.get<CodexCapability>('/tasks/codex-capability')
}

export async function assignTaskToCodex(taskUuid: string, start = false): Promise<void> {
  await apiClient.put(`/tasks/${encodeURIComponent(taskUuid)}/execution-mode`, {
    mode: 'vibe_coding',
    tool: 'codex',
    start,
  })
}

export async function loadTaskCodexContext(taskUuid: string): Promise<TaskCodexContext> {
  return apiClient.get<TaskCodexContext>(`/tasks/${encodeURIComponent(taskUuid)}/codex-context`)
}

export async function copyTaskCodexPrompt(taskUuid: string): Promise<TaskCodexContext> {
  const context = await loadTaskCodexContext(taskUuid)
  await copyText(context.prompt)
  return context
}

export async function openTaskVibeTool(taskUuid: string): Promise<CodexOpenResult> {
  let context: TaskCodexContext
  try {
    context = await loadTaskCodexContext(taskUuid)
  } catch (error) {
    return { opened: false, copied: false, error: asError(error) }
  }
  if ((context.launch || 'app') === 'terminal') {
    if (!isDesktopRuntime() || !context.exec_path || !context.tool) {
      return copyPrompt(context.prompt)
    }
    try {
      await openVibeCli({
        tool: context.tool,
        execPath: context.exec_path,
        directoryPath: context.work_dir,
        prompt: context.prompt,
        threadId: context.thread_id,
      })
      return { opened: true, copied: false }
    } catch (openError) {
      const copied = await copyPrompt(context.prompt)
      return copied.copied ? copied : { opened: false, copied: false, error: asError(openError) }
    }
  }
  return openLoadedCodex(context)
}

export async function openTaskInCodex(taskUuid: string): Promise<CodexOpenResult> {
  return openTaskVibeTool(taskUuid)
}

async function openLoadedCodex(context: TaskCodexContext): Promise<CodexOpenResult> {
  if (!isDesktopRuntime()) return copyPrompt(context.prompt)
  try {
    await openCodexThread(context.work_dir, context.prompt, context.thread_id)
    return { opened: true, copied: false }
  } catch (openError) {
    const copied = await copyPrompt(context.prompt)
    return copied.copied
      ? copied
      : { opened: false, copied: false, error: asError(openError) }
  }
}

async function copyPrompt(prompt: string): Promise<CodexOpenResult> {
  try {
    await copyText(prompt)
    return { opened: false, copied: true }
  } catch (error) {
    return { opened: false, copied: false, error: asError(error) }
  }
}

function asError(error: unknown, fallback = 'Codex operation failed'): Error {
  return error instanceof Error ? error : new Error(fallback)
}
