<script setup lang="ts">
defineOptions({ name: 'IPAMSectionTitle' });

defineProps<{
  text: string;
  count?: number;
}>();
</script>

<template>
  <h4 class="section-title">
    <span>{{ text }}</span>
    <TTag v-if="count !== undefined" size="small" variant="light">{{ count }}</TTag>
  </h4>
</template>

<style scoped>
.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 20px 0 10px;
  font-size: 13px;
  font-weight: 600;
}
</style>
