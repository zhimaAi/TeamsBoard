<script setup lang="ts">
import { computed, ref } from 'vue'
import { PlusOutlined } from '@ant-design/icons-vue'
// 本期去掉登录：团队工作入口禁用，不再读取路由 tab 参数
// import { useRoute, useRouter } from 'vue-router'
import TaskBoard from './components/TaskBoard.vue'
// 本期去掉登录：团队工作内容隐藏，恢复登录时取消注释
// import Workbench from '@/views/workbench/index.vue'
import boardViewIcon from '@/assets/board-view.svg'
import teamViewIcon from '@/assets/team-view.svg'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

type BoardTab = 'local' | 'team'
// const route = useRoute()
// const router = useRouter()
const activeTab = computed({
  // 原逻辑：route.query.tab === 'team' ? 'team' : 'local'
  // 本期去掉登录：固定停留在本地看板
  get: () => 'local' as BoardTab,
  // 原逻辑：router.replace({ query: val === 'team' ? { tab: 'team' } : {} })
  // 本期去掉登录：禁止切换到团队工作
  set: () => {},
})
const boardRef = ref<{ openCreate: () => void }>()
</script>

<template>
  <div class="board-hub">
    <header class="board-tabs">
      <div class="board-heading"><h1>{{ t('workflows.board.title') }}</h1></div>
      <div class="board-actions">
        <nav :aria-label="t('workflows.board.viewSwitch')">
          <button :class="{ active: activeTab === 'local' }" @click="activeTab='local'"><img :src="boardViewIcon" alt="" />{{ t('workflows.board.title') }}</button>
          <a-tooltip :title="t('workflows.board.comingSoon')">
            <span class="team-tab-wrapper">
              <button :class="{ active: activeTab === 'team' }" disabled><img :src="teamViewIcon" alt="" />{{ t('workflows.board.teamWork') }}</button>
            </span>
          </a-tooltip>
        </nav>
        <a-button v-if="activeTab === 'local'" class="create-task" type="primary" @click="boardRef?.openCreate()"><template #icon><PlusOutlined /></template>{{ t('workflows.board.newTask') }}</a-button>
      </div>
    </header>
    <main class="board-content" :class="{ 'team-content': activeTab === 'team' }">
      <TaskBoard v-if="activeTab === 'local'" ref="boardRef" />
      <!-- 本期去掉登录：团队工作内容隐藏，恢复登录时取消注释
      <Workbench v-else />
      -->
    </main>
  </div>
</template>

<style scoped>
.board-hub { display: flex; height: 100%; min-height: 0; flex-direction: column; background: #fff; }
.board-tabs { display: flex; height: 96px; flex: 0 0 96px; flex-direction: column; margin: 0 0 20px; padding: 0 24px; background: #fff; }
.board-heading { display: flex; min-height: 44px; flex: 0 0 44px; align-items: center; margin: 0 -24px; padding: 0 24px; border-bottom: 1px solid #f0f0f0; }
.board-tabs h1 { margin: 0; color: #262626; font-size: 20px; font-weight: 600; line-height: 28px; }
.board-actions { display: flex; height: 52px; align-items: center; justify-content: space-between; gap: 20px; }
.board-actions nav { display: flex; align-items: center; gap: 2px; padding: 2px; border-radius: 32px; background: #edeff2; }
.board-actions nav button { display: flex; align-items: center; gap: 4px; padding: 3px 12px; border: 0; border-radius: 32px; color: #595959; background: transparent; cursor: pointer; font-size: 14px; font-weight: 400; line-height: 22px; }
.board-actions nav button img { width: 16px; height: 16px; }
.board-actions nav button:not(.active) img { opacity: .76; }
.board-actions nav button.active { color: #262626; background: #fff; box-shadow: 0 1px 2px rgba(0,0,0,.08); font-weight: 600; }
.board-actions nav button.active img[src$="team-view.svg"] { filter: brightness(.43); opacity: 1; }
.board-actions nav button:disabled { color: #bfbfbf; cursor: not-allowed; }
.board-actions nav button:disabled img { opacity: .4; }
.create-task { min-width: 80px; }
.board-content { min-height: 0; flex: 1; }
.board-content:not(.team-content) { padding: 0 24px 24px; }
.team-content { overflow: hidden; border: 1px solid #e2e8f0; border-radius: 8px; background: #fff; }
.team-content :deep(.workbench-page) { min-height: 100%; }
.team-content :deep(.my-work-page) { width: 100%; height: 100%; margin: 0; }
@media (max-width: 640px) { .board-tabs { padding: 0 16px; } .board-heading { margin: 0 -16px; padding: 0 16px; } .board-content:not(.team-content) { padding: 0 16px 16px; } }
</style>
