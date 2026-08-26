<script setup lang="ts">
import { computed } from 'vue'
import darkAgent from '@/assets/icons/sidebar-nav-dark-agent.svg'
import darkApi from '@/assets/icons/sidebar-nav-dark-api.svg'
import darkCommand from '@/assets/icons/sidebar-nav-dark-command.svg'
import darkDashboardFrame from '@/assets/icons/sidebar-nav-dark-dashboard-frame.svg'
import darkDashboardGrid from '@/assets/icons/sidebar-nav-dark-dashboard-grid.svg'
import darkKnowledge from '@/assets/icons/sidebar-nav-dark-knowledge.svg'
import darkProject from '@/assets/icons/sidebar-nav-dark-project.svg'
import darkSettings from '@/assets/icons/sidebar-nav-dark-settings.svg'
import darkTaskCheck from '@/assets/icons/sidebar-nav-dark-task-check.svg'
import darkTaskFrame from '@/assets/icons/sidebar-nav-dark-task-frame.svg'
import lightAgent from '@/assets/icons/sidebar-nav-light-agent.svg'
import lightApi from '@/assets/icons/sidebar-nav-light-api.svg'
import lightCommand from '@/assets/icons/sidebar-nav-light-command.svg'
import lightDashboardFrame from '@/assets/icons/sidebar-nav-light-dashboard-frame.svg'
import lightDashboardGrid from '@/assets/icons/sidebar-nav-light-dashboard-grid.svg'
import lightKnowledge from '@/assets/icons/sidebar-nav-light-knowledge.svg'
import lightProject from '@/assets/icons/sidebar-nav-light-project.svg'
import lightSettings from '@/assets/icons/sidebar-nav-light-settings.svg'
import lightTaskCheck from '@/assets/icons/sidebar-nav-light-task-check.svg'
import lightTaskFrame from '@/assets/icons/sidebar-nav-light-task-frame.svg'

type SidebarMenuIconName =
  | 'dashboard'
  | 'task'
  | 'agent'
  | 'project'
  | 'command'
  | 'book'
  | 'api'
  | 'settings'

const props = defineProps<{
  name: SidebarMenuIconName
  dark?: boolean
}>()

const iconSources: Record<SidebarMenuIconName, { light: string[]; dark: string[] }> = {
  dashboard: {
    light: [lightDashboardFrame, lightDashboardGrid],
    dark: [darkDashboardFrame, darkDashboardGrid],
  },
  task: {
    light: [lightTaskFrame, lightTaskCheck],
    dark: [darkTaskFrame, darkTaskCheck],
  },
  agent: { light: [lightAgent], dark: [darkAgent] },
  project: { light: [lightProject], dark: [darkProject] },
  command: { light: [lightCommand], dark: [darkCommand] },
  book: { light: [lightKnowledge], dark: [darkKnowledge] },
  api: { light: [lightApi], dark: [darkApi] },
  settings: { light: [lightSettings], dark: [darkSettings] },
}

const sources = computed(() => iconSources[props.name][props.dark ? 'dark' : 'light'])
</script>

<template>
  <span class="sidebar-menu-icon" :class="props.name" aria-hidden="true">
    <img v-for="source in sources" :key="source" :src="source" alt="">
  </span>
</template>

<style scoped>
.sidebar-menu-icon {
  position: relative;
  display: inline-block;
  width: 20px;
  height: 20px;
  flex: 0 0 20px;
}

.sidebar-menu-icon img {
  position: absolute;
  display: block;
  top: 50%;
  left: 50%;
  max-width: none;
  transform: translate(-50%, -50%);
}

.task img:last-child {
  top: 58.33%;
  left: 50%;
}
</style>
