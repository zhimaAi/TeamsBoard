export interface TaskFileNode {
  name: string
  path: string
  absolute_path: string
  is_dir: boolean
  file_type?: string
  size?: number
  modified_at?: number
  owner_step_uuid?: string
  owner_step_name?: string
  children?: TaskFileNode[]
}

/**
 * 可被 # 引用的任务产出文件。
 * 只要求绝对路径可序列化；CLI 直接执行等模式不在步骤子目录下工作，
 * 这类文件没有 owner_step_uuid，但同样是任务产出，必须可被引用。
 */
export interface MentionableTaskFile extends TaskFileNode {
  absolute_path: string
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
