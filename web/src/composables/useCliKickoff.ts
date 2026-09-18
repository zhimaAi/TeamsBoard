import { ref } from 'vue'
import { useRouter } from 'vue-router'

/**
 * 需求 2204（评论 5 调整后口径）：通过三条链路把任务指派给 CLI + 模型时
 * （新建任务确定创建、看板切进行中的设置执行方式弹窗、任务详情指派执行方式），
 * 任务进入进行中后要自动向 CLI 发送对 task.md 的文档引用和一句固定提示词，
 * 由 CLI 读任务文档后开始执行——不再预填输入框等待用户手动发送。
 *
 * 意图走本模块的单例 ref 而不是路由 query：对话页的 syncConversationRoute
 * 会重写 query（只保留 taskUuid/stepUuid），走 query 的意图会被抹掉。
 */
export const pendingCliKickoffUuid = ref('')

// 开发诊断：把意图状态挂到 window，供自动化验证比对模块实例（生产无副作用）。
if (typeof window !== 'undefined') {
  ;(window as unknown as Record<string, unknown>).__kickoffModule = {
    get pending() {
      return pendingCliKickoffUuid.value
    },
    setPending(v: string) {
      pendingCliKickoffUuid.value = v
    },
  }
}

/** 任务文档的文件名，与后端写入任务目录的文件一致。 */
export const CLI_KICKOFF_DOCUMENT = 'task.md'

export interface CliKickoffPrompt {
  /** 发送内容：task.md 绝对路径 + 固定提示词，与手动插入文档引用后的提交形态一致。 */
  content: string
  documentName: string
  documentPath: string
}

/**
 * 任务文档固定落在任务目录根下（后端 task_md_path 即 taskDir/task.md）。
 *
 * 发送内容用文档引用的 marker 形态（`#[task.md] <提示词>`），与用户在输入框里
 * 插入文档引用时的显示保持一致；不要把绝对路径写进消息正文——CLI 侧并不依赖
 * 消息里的路径，后端 BuildCLIPrompt 已把「任务数据目录 + 文档绝对路径」注入 prompt。
 */
export function buildCliKickoffPrompt(
  taskDir: string,
  text: string,
): CliKickoffPrompt | null {
  const trimmedDir = String(taskDir || '').trim()
  if (!trimmedDir) return null
  const separator = trimmedDir.includes('\\') ? '\\' : '/'
  const documentPath = `${trimmedDir.replace(/[\\/]+$/, '')}${separator}${CLI_KICKOFF_DOCUMENT}`
  return {
    content: `#[${CLI_KICKOFF_DOCUMENT}] ${text}`,
    documentName: CLI_KICKOFF_DOCUMENT,
    documentPath,
  }
}

export function useCliKickoffNavigation() {
  const router = useRouter()

  return {
    /** 跳到对话页；对话页在任务就绪后自动向 CLI 发送起始指令。 */
    openCliConversation(taskUuid: string) {
      pendingCliKickoffUuid.value = taskUuid
      return router.push({ name: 'tasks', query: { taskUuid } })
    },
  }
}
