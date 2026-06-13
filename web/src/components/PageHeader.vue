<template>
  <div class="page-header">
    <div class="page-header-left">
      <h2 class="page-title">{{ title }}</h2>
      <span v-if="subtitle" class="page-subtitle">{{ subtitle }}</span>
    </div>
    <div class="page-header-right">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  subtitle?: string
}>()
</script>

<style scoped>
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}

.page-header-left {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.page-title {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
}

.page-subtitle {
  font-size: 14px;
  color: var(--n-text-color-3);
}

.page-header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

@media (max-width: 640px) {
  .page-header {
    align-items: flex-start;
    gap: 12px;
  }

  .page-header-left {
    display: block;
  }

  .page-header-right {
    flex-wrap: wrap;
    justify-content: flex-end;
  }
}
</style>
