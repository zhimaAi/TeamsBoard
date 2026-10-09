<script setup lang="ts">
import { computed } from 'vue'
import darkAgent from '@/assets/icons/sidebar-nav-dark-agent.svg'
import darkApi from '@/assets/icons/sidebar-nav-dark-api.svg'
import darkCommand from '@/assets/icons/sidebar-nav-dark-command.svg'
import darkDashboardFrame from '@/assets/icons/sidebar-nav-dark-dashboard-frame.svg'
import darkDashboardGrid from '@/assets/icons/sidebar-nav-dark-dashboard-grid.svg'
import darkKnowledge from '@/assets/icons/sidebar-nav-dark-knowledge.svg'
import darkNewConversation from '@/assets/icons/sidebar-nav-dark-new-conversation.svg'
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
import lightNewConversation from '@/assets/icons/sidebar-nav-light-new-conversation.svg'
import lightProject from '@/assets/icons/sidebar-nav-light-project.svg'
import lightSettings from '@/assets/icons/sidebar-nav-light-settings.svg'
import lightTaskCheck from '@/assets/icons/sidebar-nav-light-task-check.svg'
import lightTaskFrame from '@/assets/icons/sidebar-nav-light-task-frame.svg'

type SidebarMenuIconName =
  | 'dashboard'
  | 'newConversation'
  | 'task'
  | 'agent'
  | 'project'
  | 'command'
  | 'book'
  | 'api'
  | 'settings'
  | 'remote'

const props = defineProps<{
  name: SidebarMenuIconName
  dark?: boolean
}>()

const iconSources: Record<SidebarMenuIconName, { light: string[]; dark: string[] }> = {
  dashboard: {
    light: [lightDashboardFrame, lightDashboardGrid],
    dark: [darkDashboardFrame, darkDashboardGrid],
  },
  newConversation: {
    light: [lightNewConversation],
    dark: [darkNewConversation],
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
  remote: { light: [], dark: [] },
}

const sources = computed(() => iconSources[props.name][props.dark ? 'dark' : 'light'])
</script>

<template>
  <span class="sidebar-menu-icon" :class="props.name" aria-hidden="true">
    <svg v-if="props.name === 'remote'" width="20" height="20" viewBox="0 0 20 20" fill="none">
      <path d="M10 11.5v5.5M7.4 17h5.2M6.2 7.8a5.4 5.4 0 0 0 0 4.4m7.6-4.4a5.4 5.4 0 0 1 0 4.4M3.6 5.3a9 9 0 0 0 0 9.4m12.8-9.4a9 9 0 0 1 0 9.4" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/>
      <circle cx="10" cy="9.7" r="1.6" fill="currentColor"/>
    </svg>
    <img v-else v-for="source in sources" :key="source" :src="source" alt="">
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
