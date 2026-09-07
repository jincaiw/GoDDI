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
  margin-bottom: 28px;
  gap: 20px;
}

.page-header-left {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.page-title {
  margin: 0;
  font-size: 28px;
  font-weight: 600;
  letter-spacing: -0.8px;
  line-height: 1.3;
  color: var(--app-text);
}

.page-subtitle {
  font-size: 14px;
  color: var(--app-muted);
}

.page-header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

@media (max-width: 640px) {
  .page-header {
    flex-wrap: wrap;
    align-items: flex-start;
    gap: 12px;
  }

  .page-header-left {
    display: block;
  }
  .page-title { font-size: 24px; }

  .page-header-right {
    flex-wrap: wrap;
    justify-content: flex-end;
  }
}
</style>
