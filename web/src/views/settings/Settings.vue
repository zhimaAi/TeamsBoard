<script setup lang="ts">
import { computed, ref } from 'vue'
import { useDocumentTitle } from '@/composables/useDocumentTitle'
import GitProjects from './GitProjects.vue'
import Ssh from './Ssh.vue'
import DockerCompose from './DockerCompose.vue'
import Database from './Database.vue'

const tabTitles = {
  git: 'Git 项目',
  ssh: 'SSH',
  docker: 'Docker Compose',
  database: 'MySQL / PgSQL',
} as const

const activeTab = ref<keyof typeof tabTitles>('git')
const activeTabTitle = computed(() => tabTitles[activeTab.value])

useDocumentTitle(activeTabTitle)
</script>

<template>
  <a-tabs v-model:activeKey="activeTab" type="card" class="settings-tabs">
    <a-tab-pane key="git" tab="Git 项目">
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
