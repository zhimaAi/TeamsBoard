import { t } from '@/i18n'

/**
 * 相对时间文案，与看板卡片保持同一套措辞：刚刚 / N分钟前 / N小时前 / N天前 / 具体日期。
 * 直接读取全局 i18n 实例，避免每个调用方各写一份分支逻辑。
 */
export function formatRelativeTime(timestamp: number, locale: string): string {
  const value = timestamp < 1e12 ? timestamp * 1000 : timestamp
  const elapsed = Math.max(0, Date.now() - value)
  const minutes = Math.floor(elapsed / 60000)
  if (minutes < 1) return t('workflows.board.justNow')
  if (minutes < 60) return t('workflows.board.minutesAgo', { count: minutes })

  const hours = Math.floor(minutes / 60)
  if (hours < 24) return t('workflows.board.hoursAgo', { count: hours })

  const days = Math.floor(hours / 24)
  if (days < 30) return t('workflows.board.daysAgo', { count: days })
  return new Date(value).toLocaleDateString(locale)
}
