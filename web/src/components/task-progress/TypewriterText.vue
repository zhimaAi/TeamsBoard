<template>
  <span class="typewriter">{{ visible }}<i
    v-if="typing"
    class="typewriter-caret"
    aria-hidden="true"
  /></span>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    text: string
    /**
     * 只有「本次执行新流入」的内容才做打字机：历史回看、已完成内容直接整段渲染，
     * 否则打开旧会话会看到满屏文字同时打字。
     */
    enabled?: boolean
  }>(),
  { enabled: false },
)

const emit = defineEmits<{ done: [] }>()

// 后端按终止边界一次性定稿整段文本，不做逐 delta 上报，所以这里用固定帧数走完揭示过程：
// 一次性到达的 8000 字思考若按每帧 1 字要打上一分钟，必须按长度换算速度。
const REVEAL_FRAMES = 90
// 短文本保底每帧 2 字，避免出现「一个字一个字往外蹦」的拖沓感
const MIN_PER_FRAME = 2

const shown = ref(props.enabled ? 0 : props.text.length)
let raf = 0

const visible = computed(() =>
  shown.value >= props.text.length ? props.text : props.text.slice(0, shown.value),
)
const typing = computed(() => shown.value < props.text.length)

function stop() {
  if (raf) {
    cancelAnimationFrame(raf)
    raf = 0
  }
}

function run() {
  stop()
  const total = props.text.length
  if (shown.value >= total) return
  const perFrame = Math.max(MIN_PER_FRAME, Math.ceil(total / REVEAL_FRAMES))
  const tick = () => {
    raf = 0
    shown.value = Math.min(total, shown.value + perFrame)
    if (shown.value < total) raf = requestAnimationFrame(tick)
    else emit('done')
  }
  raf = requestAnimationFrame(tick)
}

// 同一行内容被追加（例如工具结果分段落定）时从当前位置继续揭示
watch(
  () => props.text,
  (value) => {
    if (!props.enabled) {
      stop()
      shown.value = value.length
      return
    }
    if (shown.value > value.length) shown.value = value.length
    run()
  },
)

watch(
  () => props.enabled,
  (value) => {
    // 执行结束立即给出完整内容，不让收尾被动画拖住
    if (!value) {
      stop()
      shown.value = props.text.length
    }
  },
)

if (props.enabled) run()

onBeforeUnmount(stop)
</script>

<style scoped>
.typewriter-caret {
  display: inline-block;
  width: 2px;
  height: 1em;
  margin-left: 1px;
  background: currentcolor;
  vertical-align: text-bottom;
  animation: typewriter-blink 1s step-end infinite;
}
@keyframes typewriter-blink {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0;
  }
}
</style>
