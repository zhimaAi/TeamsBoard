import { ref } from 'vue'
import { defineStore } from 'pinia'
import apiClient from '@/api/client'
import type { ExpertGroup } from '@/types/pipeline'

export const useExpertGroupStore = defineStore('expert-group', () => {
  const items = ref<ExpertGroup[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  let loadPromise: Promise<ExpertGroup[]> | undefined

  async function load(force = false) {
    if (loadPromise) return loadPromise
    if (loaded.value && !force) return items.value
    loading.value = true
    error.value = ''
    loadPromise = apiClient.get<{ items: ExpertGroup[] }>('/expert-groups')
      .then((result) => {
        items.value = (result.items || []).map((item) => ({ ...item, members: item.members || [] }))
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
    const index = items.value.findIndex((current) => current.uuid === item.uuid)
    if (index >= 0) items.value[index] = normalized
    else items.value.push(normalized)
  }

  function remove(uuid: string) {
    items.value = items.value.filter((item) => item.uuid !== uuid)
  }

  return { items, loading, loaded, error, load, upsert, remove }
})
