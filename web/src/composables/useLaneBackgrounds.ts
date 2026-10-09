import { ref } from 'vue'

/**
 * 泳道背景色按泳道 key 存在本地，默认 `#fbfbfc`。
 * 「看板」和「团队工作」共用同一个存储键，同一泳道在两个页签里显示同一种颜色，
 * 设计稿里「进行中」列的浅绿底色就是这套自定义颜色中的一个预设值。
 */
const LANE_BACKGROUND_STORAGE_KEY = 'goteams.workflows.board-lane-backgrounds.v1'
const HEX_COLOR = /^#[0-9a-f]{6}$/i

export const DEFAULT_LANE_BACKGROUND = '#fbfbfc'
export const LANE_BACKGROUND_PRESETS = ['#fff7cd', '#e7f0ff', '#e8f9ef', '#f1f2f4', DEFAULT_LANE_BACKGROUND]

export interface LaneBackgrounds {
  backgroundOf: (laneKey: string) => string
  isSelected: (laneKey: string, color: string) => boolean
  set: (laneKey: string, color: string) => void
  reset: (laneKey: string) => void
}

function readLaneBackgrounds(): Record<string, string> {
  try {
    const storedBackgrounds = JSON.parse(window.localStorage.getItem(LANE_BACKGROUND_STORAGE_KEY) || '{}') as Record<string, unknown>
    return Object.fromEntries(Object.entries(storedBackgrounds).filter((entry): entry is [string, string] => (
      typeof entry[1] === 'string' && HEX_COLOR.test(entry[1])
    )))
  } catch {
    return {}
  }
}

export function useLaneBackgrounds(): LaneBackgrounds {
  const backgrounds = ref<Record<string, string>>(readLaneBackgrounds())

  function persist() {
    try {
      window.localStorage.setItem(LANE_BACKGROUND_STORAGE_KEY, JSON.stringify(backgrounds.value))
    } catch {
      // 本地存储不可用时，仅保留当前页面会话内的颜色设置。
    }
  }

  function backgroundOf(laneKey: string) {
    return backgrounds.value[laneKey] || DEFAULT_LANE_BACKGROUND
  }

  function isSelected(laneKey: string, color: string) {
    return backgroundOf(laneKey).toLowerCase() === color.toLowerCase()
  }

  function set(laneKey: string, color: string) {
    backgrounds.value = { ...backgrounds.value, [laneKey]: color }
    persist()
  }

  function reset(laneKey: string) {
    const next = { ...backgrounds.value }
    delete next[laneKey]
    backgrounds.value = next
    persist()
  }

  return { backgroundOf, isSelected, set, reset }
}
