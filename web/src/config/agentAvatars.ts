import agentAvatar1 from '@/assets/avatars/agent-avatar-1.png'
import agentAvatar2 from '@/assets/avatars/agent-avatar-2.png'
import agentAvatar3 from '@/assets/avatars/agent-avatar-3.png'
import agentAvatar4 from '@/assets/avatars/agent-avatar-4.png'
import agentAvatar5 from '@/assets/avatars/agent-avatar-5.png'
import agentAvatar6 from '@/assets/avatars/agent-avatar-6.png'
import type { PipelineStep } from '@/types/pipeline'

export const AGENT_AVATARS = [
  agentAvatar1,
  agentAvatar2,
  agentAvatar3,
  agentAvatar4,
  agentAvatar5,
  agentAvatar6,
]

export function resolveAgentAvatar(step: Pick<PipelineStep, 'avatar'> | undefined, index = 0) {
  return step?.avatar || AGENT_AVATARS[Math.max(index, 0) % AGENT_AVATARS.length]
}
