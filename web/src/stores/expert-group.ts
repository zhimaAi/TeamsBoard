import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import apiClient from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import type { ExpertGroup } from '@/types/pipeline'

export const useExpertGroupStore = defineStore('expert-group', () => {
  const authStore = useAuthStore()
  const allItems = ref<ExpertGroup[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  let loadPromise: Promise<ExpertGroup[]> | undefined

  // 登出隐藏云端团队专家团（source_type=cloud）；本地私有专家团始终可见。
  const items = computed(() =>
    authStore.cloudLoggedIn
      ? allItems.value
      : allItems.value.filter((item) => item.source_type !== 'cloud'),
  )

  async function load(force = false) {
    if (loadPromise) return loadPromise
    if (loaded.value && !force) return items.value
    loading.value = true
    error.value = ''
    loadPromise = apiClient.get<{ items: ExpertGroup[] }>('/expert-groups')
      .then((result) => {
        allItems.value = (result.items || []).map((item) => ({ ...item, members: item.members || [] }))
        loaded.value = true
        return items.value
      })
      .catch((reason) => {
        error.value = reason instanceof Error ? reason.message : '加载专家团失败'
        throw reason
      })
      .finally(() => {
        loading.value = false
        loadPromise = undefined
      })
    return loadPromise
  }

  function upsert(item: ExpertGroup) {
    const normalized = { ...item, members: item.members || [] }
    const index = allItems.value.findIndex((current) => current.uuid === item.uuid)
    if (index >= 0) allItems.value[index] = normalized
    else allItems.value.push(normalized)
  }

  function remove(uuid: string) {
    allItems.value = allItems.value.filter((item) => item.uuid !== uuid)
  }

  return { items, loading, loaded, error, load, upsert, remove }
})
