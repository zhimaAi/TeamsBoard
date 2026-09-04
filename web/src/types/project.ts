export type ProjectIconKind =
  | 'folder'
  | 'rocket'
  | 'code'
  | 'database'
  | 'globe'
  | 'mobile'
  | 'image'
  | 'custom'

export interface LocalProject {
  uuid: string
  name: string
  icon_type: ProjectIconKind
  icon_url?: string
  local_dir: string
  created_at?: number
  updated_at?: number
}
