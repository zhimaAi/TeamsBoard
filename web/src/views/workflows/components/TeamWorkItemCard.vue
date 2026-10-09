<template>
  <div
    class="team-work-card"
    role="button"
    tabindex="0"
    @click="emit('open')"
    @keydown.enter.prevent="emit('open')"
    @keydown.space.prevent="emit('open')"
  >
    <h4 class="card-title" :title="item.title">{{ item.title }}</h4>
    <div class="card-footer">
      <span class="card-assignee">
        <img
          v-if="avatar"
          :src="avatar"
          alt=""
        />
        <span v-else class="card-assignee-fallback" aria-hidden="true">{{ initial }}</span>
        <span class="card-assignee-name" :title="assigneeName">{{ assigneeName }}</span>
      </span>
      <span v-if="item.updated_at" class="card-updated">
        {{ t('workflows.board.updatedAt', { time: relativeTime }) }}
      </span>
    </div>
    <button
      v-if="configurable"
      type="button"
      class="card-configure"
      @click.stop="emit('configure')"
    >
      {{ t('teamwork.board.configure') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { MyWorkItem } from '@/types/workitem'
import { useAppI18n } from '@/i18n'
import { useLocale } from '@/composables/useLocale'
import { formatRelativeTime } from '@/utils/relativeTime'

const { t } = useAppI18n()
const { locale } = useLocale()

const props = withDefaults(defineProps<{
  item: MyWorkItem
  /** 只有「待配置」列的卡片带「配置」按钮；已绑定本地任务的卡片只展示信息。 */
  configurable?: boolean
}>(), {
  configurable: false,
})

const emit = defineEmits<{
  open: []
  configure: []
}>()

const assigneeName = computed(() => (
  props.item.assignee_names?.[0]?.trim()
  || props.item.assignee_usernames?.[0]?.trim()
  || t('teamwork.board.unassigned')
))

const avatar = computed(() => props.item.assignee_avatars?.[0]?.trim() || '')

const initial = computed(() => assigneeName.value.slice(0, 1).toUpperCase())

const relativeTime = computed(() => (
  props.item.updated_at ? formatRelativeTime(Number(props.item.updated_at), locale.value) : ''
))
</script>

<style scoped>
.team-work-card {
  display: flex;
  width: 100%;
  box-sizing: border-box;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
  border: 1px solid #dfe5ee;
  border-radius: 13px;
  background: #fff;
  box-shadow: 0 1px 2px rgba(16,24,40,.03), 0 2px 4px rgba(34,52,79,.03);
  cursor: pointer;
  transition: box-shadow 0.2s, border-color 0.2s;
}

.team-work-card:hover {
  border-color: #c5d4f0;
  box-shadow: 0 4px 12px rgba(34, 52, 79, 0.07);
}

.team-work-card:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.card-title {
  display: -webkit-box;
  min-height: 48px;
  overflow: hidden;
  margin: 0;
  color: #262626;
  font-size: 14px;
  font-weight: 600;
  line-height: 24px;
  text-overflow: ellipsis;
  word-break: break-word;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.card-footer {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 7px;
  font-size: 12px;
  line-height: 20px;
}

.card-assignee {
  display: flex;
  min-width: 0;
  overflow: hidden;
  align-items: center;
  gap: 7px;
  color: #8c8c8c;
  white-space: nowrap;
}

.card-assignee img {
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  border: 1px solid #fff;
  border-radius: 50%;
  object-fit: cover;
}

.card-assignee-fallback {
  display: flex;
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  align-items: center;
  justify-content: center;
  border: 1px solid #fff;
  border-radius: 50%;
  color: #595959;
  background: #edeff2;
  font-size: 10px;
  line-height: 16px;
}

.card-assignee-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-updated {
  color: #8c8c8c;
  white-space: nowrap;
}

.card-configure {
  height: 32px;
  box-sizing: border-box;
  padding: 5px 16px;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  color: #595959;
  background: #fff;
  cursor: pointer;
  font-size: 14px;
  font-weight: 400;
  line-height: 22px;
  transition: border-color 0.2s, color 0.2s;
}

.card-configure:hover,
.card-configure:focus-visible {
  outline: none;
  border-color: #3157e2;
  color: #3157e2;
}
</style>
