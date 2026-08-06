export type LoginServerType = 'official' | 'custom'

export interface LastLoginConfig {
  serverType: LoginServerType
  serverUrl: string
}

const STORAGE_KEY = 'goteams_last_login'

export function useLastLogin() {
  /** Read the last login method and address; safely return null when the data is abnormal */
  function getLastLogin(): LastLoginConfig | null {
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      if (!raw) return null
      const parsed = JSON.parse(raw) as LastLoginConfig
      if (parsed?.serverType !== 'official' && parsed?.serverType !== 'custom') {
        return null
      }
      return parsed
    } catch {
      return null
    }
  }

  /** After successful login, record this login method for default selection next time */
  function saveLastLogin(config: LastLoginConfig): void {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(config))
    } catch {
      /* Ignore privacy mode / quota exception */
    }
  }

  return { getLastLogin, saveLastLogin }
}
