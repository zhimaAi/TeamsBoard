<template>
  <div class="unassigned-pipeline-guide">
    <button type="button" class="assign-guide-btn" @click="emit('assign')">
      <img class="assign-guide-icon" :src="quickAssignIcon" alt="" aria-hidden="true" />
      <span>未指派流水线，点击快速分配</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import quickAssignIcon from '@/assets/icons/pipeline-quick-assign.svg'

const emit = defineEmits<{ assign: [] }>()
</script>

<style scoped>
.unassigned-pipeline-guide {
  display: flex;
  padding: 12px 0;
}

.assign-guide-btn {
  display: inline-flex;
  min-height: 34px;
  align-items: center;
  justify-content: center;
  gap: 4px;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  padding: 5px 16px;
  color: #595959;
  background: #fff;
  cursor: pointer;
  font-size: 14px;
  line-height: 22px;
  transition:
    border-color 0.2s,
    background 0.2s,
    color 0.2s;
}

.assign-guide-btn:hover {
  border-color: #3157e2;
  background: rgba(49, 87, 226, 0.05);
  color: #3157e2;
}

.assign-guide-btn:focus-visible {
  outline: 2px solid rgba(49, 87, 226, 0.15);
  outline-offset: 2px;
}

.assign-guide-icon {
  display: block;
  width: 16px;
  height: 16px;
}
</style>
