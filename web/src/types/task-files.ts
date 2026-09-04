export interface TaskFileNode {
  name: string
  path: string
  absolute_path: string
  is_dir: boolean
  file_type?: string
  size?: number
  owner_step_uuid?: string
  owner_step_name?: string
  children?: TaskFileNode[]
}

export interface TaskFilesResponse {
  task_dir: string
  tree: TaskFileNode[]
}

export interface TaskFileContentResponse {
  path: string
  file_type: string
  size: number
  encoding?: string
  content: string
}
