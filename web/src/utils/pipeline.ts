import type { Pipeline } from '@/types/pipeline'

export function isPipelineCloud(pipeline?: Pipeline) {
  return (pipeline?.source_type || pipeline?.source) === 'cloud'
}

/** 退出云端登录后，本地缓存的团队流水线不能继续暴露给前端功能入口 */
export function filterPipelinesByCloudLogin(items: Pipeline[], cloudLoggedIn: boolean) {
  return cloudLoggedIn ? items : items.filter((item) => !isPipelineCloud(item))
}
