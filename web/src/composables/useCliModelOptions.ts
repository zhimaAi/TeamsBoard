import { computed, ref } from 'vue'
import apiClient from '@/api/client'
import type { DiscoveredCLI } from '@/views/agents/components/agentPipeline'

export function useCliModelOptions() {
  const cliOptions = ref<DiscoveredCLI[]>([])
  const modelOptions = ref<string[]>([])
  const modelCache = ref<Record<string, string[]>>({})
  const cliLoading = ref(false)
  const modelLoading = ref(false)
  let modelRequestId = 0
  const inflightModels = new Map<string, Promise<string[]>>()

  const modelCounts = computed(() => {
    const counts: Record<string, number> = {}
    for (const [cliType, models] of Object.entries(modelCache.value)) {
      counts[cliType] = models.length
    }
    return counts
  })

  async function loadCliOptions() {
    cliLoading.value = true
    try {
      const result = await apiClient.get<{ items: DiscoveredCLI[] }>('/tasks/cli-discovery')
      cliOptions.value = [...(result.items || [])].sort(
        (left, right) => Number(right.installed) - Number(left.installed),
      )
      return cliOptions.value
    } finally {
      cliLoading.value = false
    }
  }

  async function fetchModels(cliType: string) {
    const existing = inflightModels.get(cliType)
    if (existing) return existing

    const request = (async () => {
      try {
        const result = await apiClient.get<{ models: string[] }>('/tasks/cli-models', {
          cli_type: cliType,
        })
        const nextModels = result.models || []
        modelCache.value = { ...modelCache.value, [cliType]: nextModels }
        return nextModels
      } catch (error) {
        const fallback = cliOptions.value.find(item => item.type === cliType)?.models
          || modelCache.value[cliType]
          || []
        if (!fallback.length) throw error
        modelCache.value = { ...modelCache.value, [cliType]: fallback }
        return fallback
      }
    })()

    inflightModels.set(cliType, request)
    try {
      return await request
    } finally {
      inflightModels.delete(cliType)
    }
  }

  async function loadModelOptions(cliType: string) {
    const requestId = ++modelRequestId
    if (!cliType) {
      modelOptions.value = []
      modelLoading.value = false
      return []
    }

    const cached = modelCache.value[cliType]
    modelOptions.value = cached ? [...cached] : []
    modelLoading.value = !modelOptions.value.length

    try {
      const nextModels = await fetchModels(cliType)
      if (requestId === modelRequestId) modelOptions.value = nextModels
      return nextModels
    } finally {
      if (requestId === modelRequestId) modelLoading.value = false
    }
  }

  async function prefetchModelCounts(cliTypes: string[]) {
    const unique = [...new Set(cliTypes.filter(Boolean))]
    if (!unique.length) return
    await Promise.allSettled(unique.map(cliType => fetchModels(cliType)))
  }

  function resetModelOptions() {
    modelRequestId++
    modelOptions.value = []
    modelLoading.value = false
  }

  return {
    cliLoading,
    cliOptions,
    loadCliOptions,
    loadModelOptions,
    modelCounts,
    modelLoading,
    modelOptions,
    prefetchModelCounts,
    resetModelOptions,
  }
}
