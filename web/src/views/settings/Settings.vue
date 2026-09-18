<script setup lang="ts">
import { computed, ref } from 'vue'
import { useDocumentTitle } from '@/composables/useDocumentTitle'
import GitProjects from './GitProjects.vue'
import Ssh from './Ssh.vue'
import DockerCompose from './DockerCompose.vue'
import Database from './Database.vue'
import DesktopClientSettings from './DesktopClientSettings.vue'
import { isDesktopRuntime } from '@/composables/useDesktop'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

const tabTitles = computed(() => ({
  git: t('settings.gitProjects'),
  ssh: 'SSH',
  docker: 'Docker Compose',
  database: 'MySQL / PgSQL',
  desktop: t('settings.desktop'),
}))

const activeTab = ref<'git' | 'ssh' | 'docker' | 'database' | 'desktop'>('git')
const activeTabTitle = computed(() => tabTitles.value[activeTab.value])
const desktopRuntime = isDesktopRuntime()

useDocumentTitle(activeTabTitle)
</script>

<template>
  <a-tabs v-model:activeKey="activeTab" type="card" class="settings-tabs">
    <a-tab-pane key="git" :tab="t('settings.gitProjects')">
      <GitProjects />
    </a-tab-pane>
    <a-tab-pane key="ssh" tab="SSH">
      <Ssh />
    </a-tab-pane>
    <a-tab-pane key="docker" tab="Docker Compose">
      <DockerCompose />
    </a-tab-pane>
    <a-tab-pane key="database" tab="MySQL / PgSQL">
      <Database />
    </a-tab-pane>
    <a-tab-pane v-if="desktopRuntime" key="desktop" :tab="t('settings.desktop')">
      <DesktopClientSettings />
    </a-tab-pane>
  </a-tabs>
</template>

<style scoped>
.settings-tabs {
  background: #fff;
  padding: 16px;
  border-radius: 8px;
  min-height: 100%;
}
</style>
