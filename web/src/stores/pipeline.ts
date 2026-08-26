import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import apiClient from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import type { Pipeline } from '@/types/pipeline'
import { filterPipelinesByCloudLogin } from '@/utils/pipeline'

export const usePipelineStore = defineStore('pipeline', () => {
  const authStore = useAuthStore()
  const allPipelines = ref<Pipeline[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  let loadPromise: Promise<Pipeline[]> | undefined

  const pipelines = computed(() =>
    filterPipelinesByCloudLogin(allPipelines.value, authStore.cloudLoggedIn),
  )

  function normalisePipeline(pipeline: Pipeline): Pipeline {
    return { ...pipeline, steps: pipeline.steps || [] }
  }

  async function loadPipelines(force = false): Promise<Pipeline[]> {
    if (loadPromise) return loadPromise
    if (loaded.value && !force) return pipelines.value

    loading.value = true
    error.value = ''
    loadPromise = apiClient
      .get<{ items: Pipeline[] }>('/pipelines')
      .then((result) => {
        allPipelines.value = (result.items || []).map(normalisePipeline)
        loaded.value = true
        return pipelines.value
      })
      .catch((loadError) => {
        error.value = loadError instanceof Error ? loadError.message : '流水线加载失败'
        throw loadError
      })
      .finally(() => {
        loading.value = false
        loadPromise = undefined
      })

    return loadPromise
  }

  function upsertPipeline(pipeline: Pipeline) {
    const normalised = normalisePipeline(pipeline)
    const index = allPipelines.value.findIndex((item) => item.uuid === normalised.uuid)
    if (index >= 0) {
      allPipelines.value[index] = normalised
      return
    }
    allPipelines.value.push(normalised)
  }

  return {
    pipelines,
    loading,
    loaded,
    error,
    loadPipelines,
    upsertPipeline,
  }
})
