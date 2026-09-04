import type { ProjectIconKind } from '@/types/project'

export type ProjectPresetIconKind = Exclude<ProjectIconKind, 'custom'>

export interface ProjectIconPreset {
  type: ProjectPresetIconKind
  symbol: string
  color: string
  background: string
}

export const PROJECT_ICON_PRESETS: ProjectIconPreset[] = [
  { type: 'folder', symbol: '▰', color: '#3b82f6', background: '#eff6ff' },
  { type: 'rocket', symbol: '◆', color: '#f97316', background: '#fff7ed' },
  { type: 'code', symbol: '</>', color: '#22c55e', background: '#f0fdf4' },
  { type: 'database', symbol: '◉', color: '#a855f7', background: '#faf5ff' },
  { type: 'globe', symbol: '◎', color: '#06b6d4', background: '#ecfeff' },
  { type: 'mobile', symbol: '▯', color: '#ec4899', background: '#fdf2f8' },
  { type: 'image', symbol: '▧', color: '#ff0ca6', background: '#ffe6f6' },
]

export function projectIconPreset(type?: string) {
  return PROJECT_ICON_PRESETS.find((item) => item.type === type) || PROJECT_ICON_PRESETS[0]
}
