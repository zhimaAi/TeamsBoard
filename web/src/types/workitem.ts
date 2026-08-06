export interface CloudWorkItem {
  type: 'requirement' | 'defect' | 'task'
  id: number
  workspace_id: number
  workspace_name?: string
  title: string
  description?: string
}

export interface MyWorkItem extends CloudWorkItem {
  workspace_color?: string
  status_id?: number | string
  status_name?: string
  status_color?: string
  priority?: number | string
  priority_name?: string
  priority_color?: string
  priority_sort_order?: number | string
  planned_start_date?: number | string
  planned_end_date?: number | string
  assignee_ids?: Array<number | string>
  assignee_names?: string[]
  assignee_avatars?: string[]
  assignee_usernames?: string[]
  is_done?: boolean | number | string
  created_at?: number | string
  updated_at?: number | string
  local_task_uuid?: string
}
