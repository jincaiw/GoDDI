<script setup lang="ts">
import { computed } from 'vue';

/** Small trend indicator matching the official starter TopPanel footer. */
const props = withDefaults(defineProps<{ value: string; inverse?: boolean }>(), { inverse: false });

const isUp = computed(() => !props.value.startsWith('-'));
const label = computed(() => props.value.replace(/^[+-]/, ''));
</script>

<template>
  <span class="trend-tag" :class="{ 'trend-tag--inverse': inverse }">
    <TIcon :name="isUp ? 'arrow-up' : 'arrow-down'" />
    <span>{{ label }}</span>
  </span>
</template>

<style scoped>
.trend-tag {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 12px;
  color: var(--td-success-color);
}
.trend-tag :deep(.t-icon) { font-size: 14px; }
.trend-tag--inverse { color: inherit; }
</style>
