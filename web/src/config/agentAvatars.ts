// 预设头像托管在 public/avatars 下：URL 即文件名、无构建 hash，dev 与生产地址一致。
// 换头像时旧文件不删除即可保证旧数据（存的是该 URL）继续显示；覆盖同名文件则旧数据跟随新图案。
import type { PipelineStep } from '@/types/pipeline'

export const AGENT_AVATARS = [
  '/avatars/agent-avatar-1.png',
  '/avatars/agent-avatar-2.png',
  '/avatars/agent-avatar-3.png',
  '/avatars/agent-avatar-4.png',
  '/avatars/agent-avatar-5.png',
  '/avatars/agent-avatar-6.png',
  '/avatars/agent-avatar-7.png',
]

export function resolveAgentAvatar(step: Pick<PipelineStep, 'avatar'> | undefined, index = 0) {
  return step?.avatar || AGENT_AVATARS[Math.max(index, 0) % AGENT_AVATARS.length]
}
