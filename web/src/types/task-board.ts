export interface TaskBoardLane {
  id: number
  lane_key: string
  title: string
  color: string
  sort_order: number
  is_hidden: number
  task_count: number
}

export interface TaskBoardTask {
  uuid: string
  title: string
  status: string
  execution_status: string
  agent_id: string
  work_item_type: string
  work_item_id: string
  created_at: number
  updated_at: number
  content_snapshot?: string
  agent_name_snapshot?: string
  description?: string
  pipeline_snapshot_uuid?: string
  selected_pipeline_uuid?: string
  execution_mode?: import('@/types/pipeline').TaskExecutionMode
  execution_tool?: string
  // CLI 直接执行：模型由看板接口透出（若后端返回），缺省回退隐式步骤。
  execution_model?: string
  pipeline_name_snapshot?: string
  pipeline_avatar_snapshot?: string
  selected_expert_group_uuid?: string
  expert_group_snapshot_uuid?: string
  expert_group_name_snapshot?: string
  expert_group_avatar_snapshot?: string
  priority?: string
  planned_end_date?: number | string
  blocked_reason?: string
}

export type TaskViewMode = 'drawer' | 'modal' | 'new-window'
