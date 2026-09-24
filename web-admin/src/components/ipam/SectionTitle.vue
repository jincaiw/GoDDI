<script setup lang="ts">
defineProps<{
  text: string
  count?: number
}>()
</script>

<template>
  <h4 class="section-title">
    <span>{{ text }}</span>
    <NTag v-if="count !== undefined" size="tiny" :bordered="false">{{ count }}</NTag>
  </h4>
</template>

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
