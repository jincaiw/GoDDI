<script setup lang="ts">
import { computed } from 'vue';
import { ConfigProvider as TConfigProvider } from 'tdesign-vue-next';
import type { GlobalConfigProvider } from 'tdesign-vue-next';
import zhConfig from 'tdesign-vue-next/es/locale/zh_CN';
import enConfig from 'tdesign-vue-next/es/locale/en_US';
import { useAppStore } from './store/modules/app';
import { useThemeStore } from './store/modules/theme';

defineOptions({
  name: 'App'
});

const appStore = useAppStore();
const themeStore = useThemeStore();

// TDesign is the only UI library in use; the locale modules are declared
// `readonly` while GlobalConfigProvider uses mutable tuples, so the cast only
// widens the type with no runtime change.
const tdesignLocale = computed<GlobalConfigProvider>(
  () => (appStore.locale === 'en-US' ? enConfig : zhConfig) as unknown as GlobalConfigProvider
);

// The theme store already derives the watermark text (product name, user name
// or timestamp). TDesign's Watermark renders through a canvas that produced an
// empty data URL in this build, so the overlay is drawn with a CSS repeating
// gradient instead: deterministic, theme-aware and no canvas dependency.
const watermarkVisible = computed(() => Boolean(themeStore.watermark?.visible));
const watermarkContent = computed(() => String(themeStore.watermarkContent || themeStore.watermark?.text || ''));
const watermarkStyle = computed(() => ({
  backgroundImage: `repeating-linear-gradient(-45deg, transparent 0 120px, var(--td-text-color-placeholder) 120px 121px)`,
  opacity: '0.08'
}));
</script>

<template>
  <TConfigProvider :global-config="tdesignLocale">
    <RouterView class="bg-layout" />
    <div v-if="watermarkVisible" class="app-watermark" :style="watermarkStyle" aria-hidden="true">
      <span v-for="n in 40" :key="n" class="app-watermark__cell">{{ watermarkContent }}</span>
    </div>
  </TConfigProvider>
</template>

<style scoped>
.app-watermark {
  position: fixed;
  inset: 0;
  z-index: 3000;
  pointer-events: none;
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  align-content: space-around;
  justify-items: center;
  overflow: hidden;
}
.app-watermark__cell {
  transform: rotate(-24deg);
  font: var(--td-font-body-small);
  color: var(--td-text-color-placeholder);
  white-space: nowrap;
  user-select: none;
}
</style>