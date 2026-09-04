export interface CreateTaskDefaults {
  project_uuid: string
  work_dir: string
  pipeline_uuid: string
}

const STORAGE_KEY = 'goteams.createTask.lastDefaults'

function emptyDefaults(): CreateTaskDefaults {
  return { project_uuid: '', work_dir: '', pipeline_uuid: '' }
}

export function useCreateTaskDefaults() {
  function getCreateTaskDefaults(): CreateTaskDefaults {
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      if (!raw) return emptyDefaults()
      const parsed = JSON.parse(raw) as Partial<CreateTaskDefaults>
      return {
        project_uuid: typeof parsed.project_uuid === 'string' ? parsed.project_uuid : '',
        work_dir: typeof parsed.work_dir === 'string' ? parsed.work_dir : '',
        pipeline_uuid: typeof parsed.pipeline_uuid === 'string' ? parsed.pipeline_uuid : '',
      }
    } catch {
      return emptyDefaults()
    }
  }

  function saveCreateTaskDefaults(defaults: CreateTaskDefaults): void {
    try {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          project_uuid: defaults.project_uuid || '',
          work_dir: defaults.work_dir || '',
          pipeline_uuid: defaults.pipeline_uuid || '',
        }),
      )
    } catch {
      /* Ignore persistence failures in scenarios such as privacy mode */
    }
  }

  return { getCreateTaskDefaults, saveCreateTaskDefaults }
}
