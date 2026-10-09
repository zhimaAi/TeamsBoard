import codexLogo from '@/assets/icons/codex-logo.svg'
import claudeLogo from '@/assets/icons/vibe-claude.svg'
import codebuddyLogo from '@/assets/icons/vibe-codebuddy.svg'
import codexCliLogo from '@/assets/icons/vibe-codex-cli.svg'
import piLogo from '@/assets/icons/vibe-pi.svg'
import qoderLogo from '@/assets/icons/vibe-qoder.svg'

export const VIBE_CODING_TOOLS = [
  { id: 'codex', cliType: '', launch: 'app' },
  { id: 'codex_cli', cliType: 'codex', launch: 'terminal' },
  { id: 'pi', cliType: 'pi', launch: 'terminal' },
  { id: 'qoder', cliType: 'qoder', launch: 'terminal' },
  { id: 'codebuddy', cliType: 'codebuddy', launch: 'terminal' },
  { id: 'claude', cliType: 'claude', launch: 'terminal' },
] as const

export type VibeCodingToolId = (typeof VIBE_CODING_TOOLS)[number]['id']

const RECENT_VIBE_TOOL_KEY = 'goteams.vibe-coding.recent-tool'

export function isVibeCodingTool(tool: string): tool is VibeCodingToolId {
  return VIBE_CODING_TOOLS.some((item) => item.id === tool)
}

export function vibeToolLabelKey(tool: string) {
  return isVibeCodingTool(tool)
    ? `workflows.task.assign.vibeTools.${tool}`
    : 'workflows.task.assign.vibeCodingMode'
}

const VIBE_TOOL_LOGOS: Record<VibeCodingToolId, string> = {
  codex: codexLogo,
  codex_cli: codexCliLogo,
  pi: piLogo,
  qoder: qoderLogo,
  codebuddy: codebuddyLogo,
  claude: claudeLogo,
}

export function vibeToolLogo(tool: string) {
  return isVibeCodingTool(tool) ? VIBE_TOOL_LOGOS[tool] : codexLogo
}

export function readRecentVibeTool() {
  try {
    const value = localStorage.getItem(RECENT_VIBE_TOOL_KEY) || ''
    return isVibeCodingTool(value) ? value : ''
  } catch {
    return ''
  }
}

export function writeRecentVibeTool(tool: string) {
  if (!isVibeCodingTool(tool)) return
  try {
    localStorage.setItem(RECENT_VIBE_TOOL_KEY, tool)
  } catch {
    // 最近使用只影响角标，存储失败时保持当前选择可用。
  }
}
