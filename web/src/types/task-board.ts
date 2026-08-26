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
  pipeline_name_snapshot?: string
  pipeline_avatar_snapshot?: string
  priority?: string
  planned_end_date?: number | string
  blocked_reason?: string
}

export type TaskViewMode = 'drawer' | 'modal' | 'new-window'
