import { ref } from 'vue'
import apiClient from '@/api/client'

/**
 * 看板卡片这类轻量场景只需要 CLI 的展示名。
 * 这里做一次模块级缓存，避免每张卡片都各自请求一次 cli-discovery。
 */
const cliNames = ref<Record<string, string>>({})
const cliVersions = ref<Record<string, string>>({})
let pending: Promise<void> | null = null

export function useCliNames() {
  async function ensureLoaded() {
    if (Object.keys(cliNames.value).length) return
    if (!pending) {
      pending = apiClient
        .get<{ items: Array<{ type: string; name: string; version?: string }> }>('/tasks/cli-discovery')
        .then((result) => {
          const names: Record<string, string> = {}
          const versions: Record<string, string> = {}
          for (const item of result.items || []) {
            names[item.type] = item.name
            versions[item.type] = item.version || ''
          }
          cliNames.value = names
          cliVersions.value = versions
        })
        .catch(() => {
          // 拿不到展示名时回退用 type 本身，不影响卡片展示。
        })
        .finally(() => {
          pending = null
        })
    }
    await pending
  }

  function cliDisplayName(type: string) {
    return cliNames.value[type] || type
  }

  /** CLI 版本号，探测不到时返回空串（执行过程摘要行用它展示版本） */
  function cliVersion(type: string) {
    return cliVersions.value[type] || ''
  }

  return { cliNames, cliVersions, ensureLoaded, cliDisplayName, cliVersion }
}
