<template>
  <div class="lane-header">
    <span class="lane-title">
      <span class="lane-label">
        <span
          class="lane-dot"
          :style="{ background: color }"
        />
        {{ title }}
      </span>
      <span class="lane-count">{{ count }}</span>
    </span>
    <a-popover
      v-if="configurable"
      :open="colorPickerOpen"
      trigger="click"
      placement="bottomRight"
      :arrow="false"
      destroy-tooltip-on-hide
      overlay-class-name="lane-color-popover"
      @open-change="colorPickerOpen = $event"
    >
      <template #content>
        <div class="lane-color-panel">
          <p>{{ t('workflows.board.changeBackground') }}</p>
          <div class="lane-color-presets">
            <button
              v-for="preset in LANE_BACKGROUND_PRESETS"
              :key="preset"
              type="button"
              class="lane-color-swatch"
              :class="{ selected: isSelected(preset) }"
              :style="{ backgroundColor: preset }"
              :aria-label="t('workflows.board.setBackground', { color: preset })"
              :aria-pressed="isSelected(preset)"
              @click="emit('set-background', preset)"
            />
            <button type="button" class="lane-color-swatch reset-color" :aria-label="t('workflows.board.resetBackground')" @click="emit('reset-background')">
              <img :src="noColorIcon" alt="" />
            </button>
          </div>
          <div class="custom-color-row">
            <p>{{ t('workflows.board.customColor') }}</p>
            <label class="lane-color-swatch custom-color-swatch" :style="{ backgroundColor: background }">
              <input
                type="color"
                :value="background"
                :aria-label="t('workflows.board.chooseCustomBackground', { lane: title })"
                @input="emit('set-background', ($event.target as HTMLInputElement).value)"
              />
            </label>
          </div>
        </div>
      </template>
      <button type="button" class="lane-color-btn" :aria-label="t('workflows.board.setLaneBackground', { lane: title })">
        <span class="lane-more-icon" aria-hidden="true"><img :src="ellipsisDotIcon" alt="" /><img :src="ellipsisDotIcon" alt="" /><img :src="ellipsisDotIcon" alt="" /></span>
      </button>
    </a-popover>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { LANE_BACKGROUND_PRESETS } from '@/composables/useLaneBackgrounds'
import { useAppI18n } from '@/i18n'
import ellipsisDotIcon from '@/assets/ellipsis-dot.svg'
import noColorIcon from '@/assets/no-color.svg'

const { t } = useAppI18n()

const props = withDefaults(defineProps<{
  title: string
  color: string
  count: number
  background: string
  /** 虚拟泳道（如团队工作的「待配置」）同样支持改背景色，保留该开关以备后续只读场景。 */
  configurable?: boolean
}>(), {
  configurable: true,
})

const emit = defineEmits<{
  'set-background': [color: string]
  'reset-background': []
}>()

const colorPickerOpen = ref(false)

function isSelected(color: string) {
  return props.background.toLowerCase() === color.toLowerCase()
}
</script>

<style scoped>
.lane-header {
  display: flex;
  height: 50px;
  min-height: 50px;
  box-sizing: border-box;
  align-items: center;
  justify-content: space-between;
  padding: 12px 12px 11px;
}

.lane-title {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #262626;
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
}

.lane-label {
  display: flex;
  align-items: center;
  gap: 9px;
}

.lane-dot {
  width: 9px;
  min-width: 9px;
  height: 9px;
  border-radius: 4.5px;
}

.lane-count {
  display: inline-flex;
  width: 22px;
  min-width: 22px;
  height: 22px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border-radius: 11px;
  color: #8c95a8;
  background: #edeff2;
  font-size: 12px;
  font-weight: 400;
  line-height: 14px;
}

.lane-color-btn {
  display: inline-flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 6px;
  background: transparent;
  cursor: pointer;
  transition: background 0.2s;
}

.lane-color-btn:hover,
.lane-color-btn:focus-visible {
  outline: none;
  background: #e4e6eb;
}

.lane-color-btn:focus-visible,
.lane-color-swatch:focus-visible {
  box-shadow: 0 0 0 2px #3157e2;
}

.lane-more-icon {
  display: flex;
  align-items: center;
  gap: 2px;
}

.lane-more-icon img {
  width: 2px;
  height: 2px;
}

.lane-color-panel {
  width: 240px;
  box-sizing: border-box;
  padding: 16px;
}

.lane-color-panel > p,
.custom-color-row > p {
  margin: 0;
  color: #262626;
  font-size: 14px;
  line-height: 22px;
}

.lane-color-presets {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.lane-color-swatch {
  position: relative;
  display: inline-flex;
  width: 28px;
  height: 28px;
  box-sizing: border-box;
  align-items: center;
  justify-content: center;
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  cursor: pointer;
}

.lane-color-swatch.selected::after {
  position: absolute;
  inset: -5px;
  border: 2px solid #3157e2;
  border-radius: 6px;
  content: '';
  pointer-events: none;
}

.reset-color {
  padding: 0;
  background: #fff;
}

.reset-color img {
  width: 16px;
  height: 16px;
}

.custom-color-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 14px;
}

.custom-color-swatch {
  overflow: hidden;
}

.custom-color-swatch input {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  padding: 0;
  border: 0;
  opacity: 0;
  cursor: pointer;
}

:global(.lane-color-popover .ant-popover-inner) {
  padding: 0;
  border-radius: 16px;
  box-shadow: 0 6px 30px 5px rgba(0,0,0,.05), 0 16px 24px 2px rgba(0,0,0,.04), 0 8px 10px -5px rgba(0,0,0,.08);
}
</style>
