import apiClient from '@/api/client'
import { isDesktopRuntime, openCodexThread } from '@/composables/useDesktop'
import { copyText } from '@/utils/clipboard'

export interface TaskCodexContext {
  task_uuid: string
  work_dir: string
  prompt: string
  thread_id?: string
}

export interface CodexCapability {
  available: boolean
  reason_code?: string
  message?: string
}

export type CodexOpenResult =
  | { opened: true; copied: false }
  | { opened: false; copied: true }
  | { opened: false; copied: false; error: Error }

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

export async function openTaskInCodex(taskUuid: string): Promise<CodexOpenResult> {
  let context: TaskCodexContext
  try {
    context = await loadTaskCodexContext(taskUuid)
  } catch (error) {
    return { opened: false, copied: false, error: asError(error) }
  }
  if (!isDesktopRuntime()) {
    try {
      await copyText(context.prompt)
      return { opened: false, copied: true }
    } catch (error) {
      return { opened: false, copied: false, error: asError(error) }
    }
  }
  try {
    await openCodexThread(context.work_dir, context.prompt, context.thread_id)
    return { opened: true, copied: false }
  } catch (openError) {
    try {
      await copyText(context.prompt)
      return { opened: false, copied: true }
    } catch (copyError) {
      return {
        opened: false,
        copied: false,
        error: asError(copyError, asError(openError).message),
      }
    }
  }
}

function asError(error: unknown, fallback = 'Codex operation failed'): Error {
  return error instanceof Error ? error : new Error(fallback)
}
