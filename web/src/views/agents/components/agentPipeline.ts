import type { Pipeline, PipelineStep } from '@/types/pipeline'
import pipelineAvatar1 from '@/assets/avatars/pipeline-avatar-1.svg'
import pipelineAvatar2 from '@/assets/avatars/pipeline-avatar-2.svg'
import pipelineAvatar3 from '@/assets/avatars/pipeline-avatar-3.svg'
import pipelineAvatar4 from '@/assets/avatars/pipeline-avatar-4.svg'
import pipelineAvatar5 from '@/assets/avatars/pipeline-avatar-5.svg'
import pipelineAvatar6 from '@/assets/avatars/pipeline-avatar-6.svg'
import pipelineAvatar7 from '@/assets/avatars/pipeline-avatar-7.svg'

export { AGENT_AVATARS, resolveAgentAvatar } from '@/config/agentAvatars'
export { isPipelineCloud } from '@/utils/pipeline'

export interface DiscoveredCLI {
  type: string
  name: string
  installed: boolean
  version?: string
  models?: string[]
}

export interface PipelineInput {
  name: string
  description: string
  avatar: string
}

export interface StepInput {
  name: string
  description: string
  avatar: string
  prompt: string
  cli_type: string
  model_name: string
}

export interface CloudStepExecutionInput {
  cli_type: string
  model_name: string
}

export interface DeleteResponse {
  deleted: boolean
}

export interface ReorderResponse {
  ok: boolean
}

export interface ReusablePipelineStep extends PipelineStep {
  pipelineName: string
}

export const PIPELINE_AVATARS = [
  pipelineAvatar1,
  pipelineAvatar2,
  pipelineAvatar3,
  pipelineAvatar4,
  pipelineAvatar5,
  pipelineAvatar6,
  pipelineAvatar7,
]

export function stepModel(step: PipelineStep) {
  return step.model_name || step.model || ''
}

export function missingSteps(pipeline: Pipeline) {
  return (pipeline.steps || []).filter((step) => !step.cli_type || !stepModel(step))
}

export function sortPipelineSteps(steps: PipelineStep[]) {
  return [...steps].sort((a, b) => a.sort_order - b.sort_order)
}
