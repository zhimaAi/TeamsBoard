export type PipelineSource = 'local' | 'cloud'
export type StepBusinessStatus = 'pending' | 'active' | 'completed'
export type ExecutionStatus = 'idle' | 'created' | 'running' | 'success' | 'failed' | 'stopped' | 'interrupted' | ''

export interface ConversationRuntimeConfig {
  cli_type: string
  model_name: string
}

export interface PipelineStep {
  uuid: string
  pipeline_uuid?: string
  source_step_id?: string | number
  local_step_uuid?: string
  cloud_step_id?: string | number
  step_key?: string
  name: string
  description?: string
  avatar?: string
  prompt?: string
  prompt_snapshot?: string
  cli_type: string
  model?: string
  model_name?: string
  sort_order: number
  status?: StepBusinessStatus
  execution_status?: ExecutionStatus
  step_dir?: string
}

export interface Pipeline {
  uuid: string
  name: string
  description?: string
  source?: PipelineSource
  source_type?: PipelineSource
  avatar?: string
  enabled?: boolean | number
  steps?: PipelineStep[]
  step_count?: number
  created_at?: number
  updated_at?: number
}

export interface TaskProgress {
  uuid: string
  task_uuid: string
  task_step_uuid: string
  record_type?: string
  user_prompt?: string
  question?: string
  status: ExecutionStatus
  final_result?: string
  result?: string
  cli_type?: string
  model?: string
  session_uuid?: string
  latest_event_type?: string
  latest_event_content?: string
  latest_event_at?: number
  created_at: number
  started_at?: number
  finished_at?: number
}

export interface CompleteStepResponse {
  task_done: boolean
  next_step_uuid?: string
  session_uuid?: string
  start_error?: string
  auto_started: boolean
}

export interface LocalTask {
  uuid: string
  title: string
  description?: string
  status: string
  execution_status?: ExecutionStatus
  source_type?: PipelineSource
  work_item_type?: string
  work_item_id?: string | number
  task_dir?: string
  work_dir?: string
  work_dirs?: string[]
  current_step_uuid?: string
  current_step_completed?: boolean | number
  pipeline_snapshot_uuid?: string
  pipeline_name_snapshot?: string
  created_at: number
  updated_at: number
  steps: PipelineStep[]
}

export interface TaskNotification {
  uuid: string
  task_uuid: string
  task_title?: string
  title?: string
  task_step_uuid: string
  step_name?: string
  step_sort_order?: number
  step_order?: number
  progress_uuid?: string
  status?: ExecutionStatus
  terminal_status?: ExecutionStatus
  summary?: string
  is_read?: boolean | number
  created_at: number
}
