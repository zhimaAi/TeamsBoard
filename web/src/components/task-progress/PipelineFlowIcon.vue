<template>
  <img class="pipeline-flow-icon" :src="pipelineFlowIcon" alt="" aria-hidden="true" />
</template>

<script setup lang="ts">
import pipelineFlowIcon from '@/assets/icons/pipeline-flow.svg'
</script>

<style scoped>
.pipeline-flow-icon {
  display: block;
  width: 16px;
  height: 16px;
}
</style>
