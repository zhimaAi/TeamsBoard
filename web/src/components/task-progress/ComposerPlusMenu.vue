<template>
  <div class="composer-plus-menu" ref="menuRef">
    <button
      ref="triggerRef"
      type="button"
      class="plus-trigger"
      :class="{ 'is-active': open }"
      :disabled="disabled"
      :title="t('workflows.task.progress.plusMenuTrigger')"
      :aria-label="t('workflows.task.progress.plusMenuTrigger')"
      :aria-expanded="open"
      aria-haspopup="menu"
      @click="toggleMenu"
    >
      <PlusOutlined />
    </button>
    <Teleport to="body">
      <Transition name="menu-fade">
        <div
          v-if="open"
          ref="dropdownRef"
          class="plus-dropdown"
          role="menu"
          :aria-label="t('workflows.task.progress.plusMenuTrigger')"
          :style="dropdownStyle"
        >
          <!-- 顺序：知识库、产出文档、附件 -->
          <button
            v-if="showKnowledge"
            type="button"
            class="plus-menu-item"
            role="menuitem"
            @click="handleKnowledge"
          >
            <img
              class="menu-item-icon"
              :src="knowledgeIcon"
              alt=""
              aria-hidden="true"
            />
            <span>{{ t('workflows.task.progress.knowledge') }}</span>
          </button>
          <button
            v-if="showTaskDocuments"
            type="button"
            class="plus-menu-item"
            role="menuitem"
            @click="handleDocuments"
          >
            <img
              class="menu-item-icon"
              :src="documentIcon"
              alt=""
              aria-hidden="true"
            />
            <span>{{ t('workflows.task.progress.documents') }}</span>
          </button>
          <button
            type="button"
            class="plus-menu-item"
            role="menuitem"
            @click="handleAttachment"
          >
            <PaperClipOutlined class="menu-item-icon" />
            <span>{{ t('workflows.task.progress.attachment') }}</span>
          </button>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { PlusOutlined, PaperClipOutlined } from '@ant-design/icons-vue'
import documentIcon from '@/assets/icons/task-composer-document.svg'
import knowledgeIcon from '@/assets/icons/sidebar-nav-light-knowledge.svg'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()

const props = withDefaults(
  defineProps<{
    disabled?: boolean
    showTaskDocuments?: boolean
    showKnowledge?: boolean
  }>(),
  {
    disabled: false,
    showTaskDocuments: true,
    showKnowledge: true,
  },
)

const emit = defineEmits<{
  attachment: []
  documents: []
  knowledge: []
}>()

const open = ref(false)
const menuRef = ref<HTMLElement>()
const triggerRef = ref<HTMLElement>()
const dropdownRef = ref<HTMLElement>()
const dropdownLeft = ref(0)
const dropdownTop = ref(0)

/**
 * 使用 fixed 定位 + JS 计算坐标，绕过 .composer-shell 的 overflow: hidden 裁剪。
 * 按钮在工具栏底部、下面就是输入框，没有足够空间向下展开，所以默认向上弹出。
 */
const dropdownStyle = computed(() => ({
  left: `${dropdownLeft.value}px`,
  top: `${dropdownTop.value}px`,
}))

function updateDropdownPosition() {
  const trigger = triggerRef.value
  const dropdown = dropdownRef.value
  if (!trigger || !dropdown) return
  const rect = trigger.getBoundingClientRect()
  const dropdownWidth = dropdown.offsetWidth
  const dropdownHeight = dropdown.offsetHeight
  // 水平方向：默认弹窗左边缘对齐按钮左边缘；若会超出右视口，则改为右对齐
  let left = rect.left
  if (left + dropdownWidth > window.innerWidth - 4) {
    left = rect.right - dropdownWidth
  }
  if (left < 4) left = 4
  dropdownLeft.value = left
  // 垂直方向：优先向上弹出；视口顶部空间不足时回退到按钮下方
  const upwardTop = rect.top - dropdownHeight - 6
  if (upwardTop >= 4) {
    dropdownTop.value = upwardTop
  } else {
    dropdownTop.value = rect.bottom + 6
  }
}

function toggleMenu() {
  if (props.disabled) return
  open.value = !open.value
  if (open.value) {
    // 双重 nextTick：先等 v-if 渲染、再等 dropdown 完成布局与样式应用，
    // 避免读到 offsetWidth/offsetHeight = 0 的过渡帧。
    nextTick(() => {
      nextTick(updateDropdownPosition)
    })
  }
}

function closeMenu() {
  open.value = false
}

function handleAttachment() {
  closeMenu()
  emit('attachment')
}

function handleDocuments() {
  closeMenu()
  emit('documents')
}

function handleKnowledge() {
  closeMenu()
  emit('knowledge')
}

/**
 * 点击组件外部关闭菜单；trigger 与 dropdown 都在 menuRef 内，
 * 内部点击通过 contains 判断不触发关闭逻辑。
 */
function handleDocumentClick(event: MouseEvent) {
  if (!open.value) return
  const target = event.target as Node | null
  if (!target) return
  if (menuRef.value && menuRef.value.contains(target)) return
  closeMenu()
}

/**
 * ESC 键关闭菜单。无任务上下文（disabled=true）的状态下不会打开，
 * 所以这里不必再校验 disabled。
 */
function handleDocumentKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && open.value) {
    closeMenu()
  }
}

/**
 * 窗口滚动或 resize 时重新定位弹窗，避免视口位置变化后弹窗跑到别处。
 */
function handleScroll() {
  if (open.value) updateDropdownPosition()
}

onMounted(() => {
  document.addEventListener('click', handleDocumentClick)
  document.addEventListener('keydown', handleDocumentKeydown)
  window.addEventListener('scroll', handleScroll, true)
  window.addEventListener('resize', handleScroll)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleDocumentClick)
  document.removeEventListener('keydown', handleDocumentKeydown)
  window.removeEventListener('scroll', handleScroll, true)
  window.removeEventListener('resize', handleScroll)
})
</script>

<style scoped>
.composer-plus-menu {
  position: relative;
  display: inline-flex;
  align-items: center;
  z-index: 10;
}

.plus-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: 6px;
  padding: 0;
  color: #595959;
  background: transparent;
  cursor: pointer;
  font-size: 18px;
  transition: background-color 0.15s, color 0.15s;
}

.plus-trigger:hover:not(:disabled) {
  background: #f2f4f7;
  color: #262626;
}

.plus-trigger.is-active {
  background: #f2f4f7;
  color: #3157e2;
}

.plus-trigger:disabled {
  color: #bfbfbf;
  cursor: not-allowed;
}

.plus-trigger:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.plus-dropdown {
  position: fixed;
  min-width: 160px;
  padding: 6px;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  background: #fff;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12);
  z-index: 1000;
}

.plus-menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 8px 10px;
  border: 0;
  border-radius: 6px;
  color: #262626;
  background: transparent;
  cursor: pointer;
  font-family: inherit;
  font-size: 14px;
  line-height: 22px;
  text-align: left;
  transition: background-color 0.15s;
}

.plus-menu-item:hover {
  background: #f2f4f7;
}

.plus-menu-item:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: -2px;
}

.menu-item-icon {
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  object-fit: contain;
  color: #595959;
}

/* 菜单动画 */
.menu-fade-enter-active,
.menu-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.menu-fade-enter-from,
.menu-fade-leave-to {
  opacity: 0;
  transform: translateY(4px);
}
</style>
