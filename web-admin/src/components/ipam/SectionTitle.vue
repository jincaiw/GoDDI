<template>
  <h4 class="section-title">
    <span>{{ text }}</span>
    <n-tag v-if="count !== undefined" size="tiny" :bordered="false">{{ count }}</n-tag>
  </h4>
</template>

<script setup lang="ts">
defineProps<{
  text: string
  count?: number
}>()
</script>

<style scoped>
.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 20px 0 10px;
  font-size: 13px;
  font-weight: 500;
}
</style>
