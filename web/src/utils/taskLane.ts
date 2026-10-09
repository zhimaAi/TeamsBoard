import { t } from '@/i18n'
import type { TaskBoardLane } from '@/types/task-board'

/** 任务状态归一到泳道 key：todo 归入 pending，执行中的中间态归入 active。 */
export function taskLaneKey(status: string): string {
  if (status === 'todo') return 'pending'
  if (['in_progress', 'running', 'developing', 'developed'].includes(status)) return 'active'
  if (status === 'completed') return 'done'
  return status
}

/**
 * 本地泳道标题统一走词条，数据库里的 title 只作为自定义泳道的兜底，
 * 这样中英文界面下四个内置泳道都不会漏出中文。
 */
export function taskLaneTitle(lane: TaskBoardLane): string {
  const key = lane.lane_key.toLowerCase()
  if (['pending', 'todo'].includes(key) || ['待开始', '待启动', '待规划'].includes(lane.title)) {
    return t('workflows.task.status.pending')
  }
  if (['active', 'in_progress', 'running', 'developing', 'developed'].includes(key) || lane.title === '进行中') {
    return t('workflows.task.status.inProgress')
  }
  if (key === 'blocked' || lane.title === '已阻塞') {
    return t('workflows.task.status.blocked')
  }
  if (['done', 'completed'].includes(key) || ['已完成', '完成'].includes(lane.title)) {
    return t('workflows.task.status.done')
  }
  return lane.title
}
