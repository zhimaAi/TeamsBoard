import type { PipelineStep, SessionEventItem, TaskExecutionMode, TaskProgress } from './pipeline'

export interface TaskExecutionHistorySummary {
  uuid: string
  task_uuid: string
  execution_no: number
  execution_mode: TaskExecutionMode
  execution_tool: string
  target_name: string
  model_name: string
  started_at: number
  archived_at: number
}

export interface TaskExecutionHistoryDetail extends TaskExecutionHistorySummary {
  version: number
  steps: PipelineStep[]
  progress: TaskProgress[]
  session_events: Record<string, SessionEventItem[]>
}
