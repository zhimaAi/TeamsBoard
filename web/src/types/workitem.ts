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
  /** 云端工作项创建人姓名，「我的工作」列表原样带回。 */
  creator_name?: string
  /** 已绑定本地任务时由后端回填的任务 uuid；未绑定的工作项没有该字段。 */
  local_task_uuid?: string
  /** 已绑定本地任务时由后端回填的任务状态，取值同看板泳道：pending/active/done/blocked。 */
  local_task_status?: string
}
