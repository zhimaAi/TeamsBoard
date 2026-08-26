import type { LocalTask } from '@/types/pipeline'

export interface TaskRelatedProject {
  uuid: string
  name: string
  icon?: string
  icon_type?: string
  main_dir?: string
  local_dir?: string
  relation_type?: string
  sort_order?: number
}

export type TaskWithDetails = LocalTask & {
  selected_pipeline_uuid?: string
  pipeline_avatar_snapshot?: string
  project_name?: string
  project_uuid?: string
  project_icon?: string
  projects?: TaskRelatedProject[]
  priority?: string
  planned_start_date?: string | number
  planned_end_date?: string | number
}
