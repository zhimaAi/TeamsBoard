<template>
  <div class="next-step-control" :style="positionStyle">
    <a-tooltip
      title="阶段已完成，进入下一步"
      placement="left"
      :open="tooltipOpen"
      @open-change="handleTooltipOpenChange"
    >
      <button
        type="button"
        class="next-step-button"
        :class="{ dragging }"
        :disabled="disabled"
        :aria-label="accessibleLabel"
        :aria-busy="completing"
        aria-keyshortcuts="ArrowUp ArrowDown ArrowLeft ArrowRight"
        :title="tipVisible ? '可拖动；使用方向键调整位置' : undefined"
        @click="handleClick"
        @keydown="handleKeydown"
        @pointerdown="handlePointerDown"
        @pointermove="handlePointerMove"
        @pointerup="finishPointerDrag"
        @pointercancel="finishPointerDrag"
      >
        <img :src="nextStepIcon" alt="" aria-hidden="true" draggable="false" />
      </button>
    </a-tooltip>

    <div v-if="tipVisible" class="next-step-tip" role="status">
      <button type="button" class="next-step-tip-close" aria-label="关闭提示" @click="dismissTip">
        <CloseOutlined aria-hidden="true" />
      </button>
      <span>当前阶段完成后，点此进入下一步</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { CloseOutlined } from '@ant-design/icons-vue'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import nextStepIcon from '@/assets/icons/task-next-step.svg'

interface Position {
  left: number
  top: number
}

interface DragState extends Position {
  pointerId: number
  pointerX: number
  pointerY: number
}

const BUTTON_SIZE = 32
const VIEWPORT_GAP = 8
const DRAG_THRESHOLD = 4
const KEYBOARD_STEP = 8
const TIP_DISMISSED_STORAGE_KEY = 'goteams-next-step-tip-dismissed'

const props = withDefaults(
  defineProps<{
    nextStepName?: string
    isLastStep: boolean
    disabled?: boolean
    completing?: boolean
  }>(),
  { nextStepName: '', disabled: false, completing: false },
)

const emit = defineEmits<{
  confirm: []
}>()

const position = ref<Position>()
const dragging = ref(false)
const tipVisible = ref(window.localStorage.getItem(TIP_DISMISSED_STORAGE_KEY) !== 'true')
const tooltipOpen = ref(false)
let dragState: DragState | undefined
let suppressNextClick = false

const positionStyle = computed(() =>
  position.value
    ? {
        left: `${position.value.left}px`,
        top: `${position.value.top}px`,
        right: 'auto',
      }
    : undefined,
)

const accessibleLabel = computed(() => {
  if (props.completing) return '处理中'
  if (props.isLastStep) return '完成任务'
  return props.nextStepName ? `进入下一步：${props.nextStepName}` : '进入下一步'
})

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), Math.max(min, max))
}

function setPosition(left: number, top: number) {
  position.value = {
    left: clamp(left, VIEWPORT_GAP, window.innerWidth - BUTTON_SIZE - VIEWPORT_GAP),
    top: clamp(top, VIEWPORT_GAP, window.innerHeight - BUTTON_SIZE - VIEWPORT_GAP),
  }
}

function currentPosition(target: HTMLButtonElement): Position {
  const rect = target.getBoundingClientRect()
  return { left: rect.left, top: rect.top }
}

function handlePointerDown(event: PointerEvent) {
  if (props.disabled || event.button !== 0) return
  tooltipOpen.value = false
  const target = event.currentTarget as HTMLButtonElement
  const origin = currentPosition(target)
  dragState = {
    ...origin,
    pointerId: event.pointerId,
    pointerX: event.clientX,
    pointerY: event.clientY,
  }
  dragging.value = false
  suppressNextClick = false
  target.setPointerCapture(event.pointerId)
}

function handlePointerMove(event: PointerEvent) {
  if (!dragState || dragState.pointerId !== event.pointerId) return
  const offsetX = event.clientX - dragState.pointerX
  const offsetY = event.clientY - dragState.pointerY
  if (!dragging.value && Math.hypot(offsetX, offsetY) < DRAG_THRESHOLD) return
  event.preventDefault()
  dragging.value = true
  suppressNextClick = true
  setPosition(dragState.left + offsetX, dragState.top + offsetY)
}

function finishPointerDrag(event: PointerEvent) {
  if (!dragState || dragState.pointerId !== event.pointerId) return
  const target = event.currentTarget as HTMLButtonElement
  if (target.hasPointerCapture(event.pointerId)) target.releasePointerCapture(event.pointerId)
  dragState = undefined
  dragging.value = false
}

function handleClick(event: MouseEvent) {
  if (suppressNextClick && event.detail > 0) {
    suppressNextClick = false
    return
  }
  suppressNextClick = false
  emit('confirm')
}

function dismissTip() {
  tipVisible.value = false
  window.localStorage.setItem(TIP_DISMISSED_STORAGE_KEY, 'true')
}

function handleTooltipOpenChange(open: boolean) {
  tooltipOpen.value = open && !tipVisible.value && !dragging.value
}

function handleKeydown(event: KeyboardEvent) {
  const offsets: Record<string, Position> = {
    ArrowUp: { left: 0, top: -KEYBOARD_STEP },
    ArrowDown: { left: 0, top: KEYBOARD_STEP },
    ArrowLeft: { left: -KEYBOARD_STEP, top: 0 },
    ArrowRight: { left: KEYBOARD_STEP, top: 0 },
  }
  const offset = offsets[event.key]
  if (!offset) return
  event.preventDefault()
  const target = event.currentTarget as HTMLButtonElement
  const current = position.value || currentPosition(target)
  setPosition(current.left + offset.left, current.top + offset.top)
}

function keepPositionInViewport() {
  if (position.value) setPosition(position.value.left, position.value.top)
}

onMounted(() => window.addEventListener('resize', keepPositionInViewport))
onBeforeUnmount(() => window.removeEventListener('resize', keepPositionInViewport))
</script>

<style scoped>
.next-step-control {
  position: fixed;
  z-index: 3;
  top: 200px;
  right: 24px;
  width: 32px;
  height: 32px;
}

.next-step-button {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 40px;
  padding: 0;
  background: #000;
  box-shadow: 0 4px 16px 0 rgba(0, 0, 0, 0.08);
  cursor: grab;
  touch-action: none;
  user-select: none;
  transition: opacity 180ms ease;
}

.next-step-button:hover:not(:disabled) {
  opacity: 0.84;
}

.next-step-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.next-step-button.dragging {
  cursor: grabbing;
  opacity: 0.84;
}

.next-step-button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.next-step-button img {
  display: block;
  width: 20px;
  height: 20px;
}

.next-step-tip {
  position: absolute;
  top: 48px;
  right: 0;
  display: flex;
  box-sizing: border-box;
  width: 244px;
  max-width: calc(100vw - 40px);
  min-height: 54px;
  align-items: center;
  border-radius: 12px;
  padding: 16px;
  background: #fff;
  box-shadow:
    0 8px 10px -5px rgba(0, 0, 0, 0.08),
    0 16px 24px 2px rgba(0, 0, 0, 0.04),
    0 6px 30px 5px rgba(0, 0, 0, 0.05);
  color: #262626;
  font-size: 14px;
  font-weight: 400;
  line-height: 22px;
  overflow-wrap: anywhere;
}

.next-step-tip-close {
  position: absolute;
  top: -8px;
  left: -8px;
  display: inline-flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 32px;
  padding: 0;
  background: rgba(0, 0, 0, 0.45);
  color: #fff;
  cursor: pointer;
  font-size: 12px;
}

.next-step-tip-close:hover {
  background: rgba(0, 0, 0, 0.58);
}

.next-step-tip-close:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

@media (prefers-reduced-motion: reduce) {
  .next-step-button {
    transition: none;
  }
}
</style>
