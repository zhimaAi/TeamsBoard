import { defineStore } from 'pinia'
import { ref } from 'vue'

/** Cloud connection status */
export type CloudStatus = 'online' | 'offline' | 'auth-expired'

/** Manageable left navigation menu key (consistent with backend tools.MenuKeys) */
export const MANAGEABLE_MENU_KEYS = [
  'workflows',
  'tasks',
  'agents',
  'projects',
  'commands',
  'knowledge',
  'apis',
  'settings',
] as const

export interface MenuConfigItem {
  key: string
  enabled: boolean
  sort_order: number
}

export const useAppStore = defineStore('app', () => {
  /** Cloud connection status */
  const cloudStatus = ref<CloudStatus>('offline')

  /** Number of CLI running tasks */
  const cliTaskCount = ref(0)

  /** Number of unread task notifications */
  const unreadTaskNotifications = ref(0)

  /** Left navigation menu switch status (key -> whether it is visible), visible by default if not recorded */
  const menuEnabled = ref<Record<string, boolean>>({})

  /** Left navigation menu order; tool center is fixed at the bottom and does not participate in sorting */
  const menuOrder = ref<string[]>([...MANAGEABLE_MENU_KEYS])

  /** Set cloud connection status */
  function setCloudStatus(status: CloudStatus) {
    cloudStatus.value = status
  }

  /** Set the number of CLI running tasks */
  function setCliTaskCount(count: number) {
    cliTaskCount.value = Math.max(0, count)
  }

  /** Set the number of unread task notifications */
  function setUnreadTaskNotifications(count: number) {
    unreadTaskNotifications.value = Math.max(0, count)
  }

  /** Batch set menu switch status */
  function setMenuEnabled(map: Record<string, boolean>) {
    menuEnabled.value = { ...map }
  }

  /** Simultaneously apply the visibility and sorting configuration returned by the backend */
  function setMenuConfig(items: MenuConfigItem[]) {
    const manageableKeys = new Set<string>(MANAGEABLE_MENU_KEYS)
    const sortedItems = [...items]
      .filter((item) => manageableKeys.has(item.key))
      .sort((a, b) => a.sort_order - b.sort_order)

    const enabled: Record<string, boolean> = {}
    const orderedKeys: string[] = []
    for (const item of sortedItems) {
      if (!orderedKeys.includes(item.key)) {
        orderedKeys.push(item.key)
        enabled[item.key] = item.enabled
      }
    }
    for (const key of MANAGEABLE_MENU_KEYS) {
      if (!orderedKeys.includes(key)) orderedKeys.push(key)
    }

    menuEnabled.value = enabled
    menuOrder.value = orderedKeys
  }

  /** Optimistically update the menu order, the left menu will be synchronized immediately when dragging */
  function setMenuOrder(keys: string[]) {
    const manageableKeys = new Set<string>(MANAGEABLE_MENU_KEYS)
    const requested = new Set(keys)
    menuOrder.value = [
      ...keys.filter((key, index) => keys.indexOf(key) === index && manageableKeys.has(key)),
      ...MANAGEABLE_MENU_KEYS.filter((key) => !requested.has(key)),
    ]
  }

  /** Determine whether a menu is visible (visible by default if not configured) */
  function isMenuVisible(key: string): boolean {
    return menuEnabled.value[key] !== false
  }

  return {
    cloudStatus,
    cliTaskCount,
    unreadTaskNotifications,
    menuEnabled,
    menuOrder,
    setCloudStatus,
    setCliTaskCount,
    setUnreadTaskNotifications,
    setMenuEnabled,
    setMenuConfig,
    setMenuOrder,
    isMenuVisible,
  }
})

