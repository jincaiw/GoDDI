<script setup lang="ts">
import { computed } from 'vue';
import { useAppStore } from '@/store/modules/app';
import { useThemeStore } from '@/store/modules/theme';
import { $t } from '@/locales';

defineOptions({ name: 'StarterSettingDrawer' });

const appStore = useAppStore();
const themeStore = useThemeStore();

const schemeOptions = computed(() => [
  { label: $t('theme.appearance.themeSchema.light'), value: 'light' },
  { label: $t('theme.appearance.themeSchema.dark'), value: 'dark' },
  { label: $t('theme.appearance.themeSchema.auto'), value: 'auto' }
]);

const colorOptions = computed(() => [
  { label: $t('theme.appearance.themeColor.primary'), value: '#0052D9' },
  { label: $t('theme.appearance.themeColor.info'), value: '#2080F0' },
  { label: $t('theme.appearance.themeColor.success'), value: '#00A870' },
  { label: $t('theme.appearance.themeColor.warning'), value: '#ED7B2F' },
  { label: $t('theme.appearance.themeColor.error'), value: '#E34D59' }
]);

const primaryColor = computed({
  get: () => String(themeStore.themeColor || '#0052D9'),
  set: (value: string) => themeStore.updateThemeColors('primary', value)
});

const layoutOptions = computed(() => [
  { label: $t('starter.layoutVertical'), value: 'vertical' },
  { label: $t('starter.layoutHorizontal'), value: 'horizontal' }
]);

const layout = computed({
  get: () => String(themeStore.layout?.mode || 'vertical'),
  set: (value: string) => themeStore.setThemeLayout(value as UnionKey.ThemeLayoutMode)
});
const scheme = computed({
  get: () => String(themeStore.themeScheme),
  set: (value: string) => themeStore.setThemeScheme(value as UnionKey.ThemeScheme)
});

const radius = computed({
  get: () => Number(themeStore.themeRadius ?? 6),
  set: (value: number) => themeStore.setThemeRadius(value)
});

const radiusOptions = computed(() => [
  { label: '0', value: 0 },
  { label: '2', value: 2 },
  { label: '6', value: 6 },
  { label: '12', value: 12 }
]);

const breadcrumbVisible = computed({
  get: () => Boolean(themeStore.header?.breadcrumb?.visible),
  set: (value: boolean) => themeStore.setBreadcrumbVisible(value)
});
const tabVisible = computed({
  get: () => Boolean(themeStore.tab?.visible),
  set: (value: boolean) => themeStore.setTabVisible(value)
});
const footerVisible = computed({
  get: () => Boolean(themeStore.footer?.visible),
  set: (value: boolean) => themeStore.setFooterVisible(value)
});
const fixedHeader = computed({
  get: () => Boolean(themeStore.fixedHeaderAndTab),
  set: (value: boolean) => themeStore.setFixedHeaderAndTab(value)
});

const watermarkVisible = computed({
  get: () => Boolean(themeStore.watermark?.visible),
  set: (value: boolean) => themeStore.setWatermarkVisible(value)
});

const grayscale = computed({
  get: () => Boolean(themeStore.grayscale),
  set: (value: boolean) => themeStore.setGrayscale(value)
});

const colourWeakness = computed({
  get: () => Boolean(themeStore.colourWeakness),
  set: (value: boolean) => themeStore.setColourWeakness(value)
});

function close() {
  appStore.closeThemeDrawer();
}
</script>

<template>
  <TDrawer
    v-if="appStore.themeDrawerVisible"
    :visible="appStore.themeDrawerVisible"
    :header="$t('theme.themeDrawerTitle')"
    placement="right"
    size="360px"
    :footer="false"
    @update:visible="(visible: boolean) => !visible && close()"
  >
    <div class="starter-setting-section">
      <div class="starter-setting-label">{{ $t('theme.appearance.themeSchema.title') }}</div>
      <TSelect v-model="scheme" :options="schemeOptions" />
    </div>
    <div class="starter-setting-section">
      <div class="starter-setting-label">{{ $t('theme.appearance.themeColor.title') }}</div>
      <TSelect v-model="primaryColor" :options="colorOptions" />
    </div>
    <div class="starter-setting-section">
      <div class="starter-setting-label">{{ $t('starter.layout') }}</div>
      <TSelect v-model="layout" :options="layoutOptions" />
    </div>
    <div class="starter-setting-section">
      <div class="starter-setting-label">{{ $t('theme.appearance.themeRadius.title') }}</div>
      <TSelect v-model="radius" :options="radiusOptions" />
    </div>
    <div class="starter-setting-divider">{{ $t('theme.appearance.elementSwitch') }}</div>
    <div class="starter-setting-section">
      <div class="starter-setting-label">{{ $t('theme.appearance.showBreadcrumb') }}</div>
      <TSwitch v-model="breadcrumbVisible" />
    </div>
    <div class="starter-setting-section">
      <div class="starter-setting-label">{{ $t('theme.appearance.useTagTabs') }}</div>
      <TSwitch v-model="tabVisible" />
    </div>
    <div class="starter-setting-section">
      <div class="starter-setting-label">{{ $t('theme.appearance.showFooter') }}</div>
      <TSwitch v-model="footerVisible" />
    </div>
    <div class="starter-setting-section">
      <div class="starter-setting-label">{{ $t('theme.appearance.fixedHeader') }}</div>
      <TSwitch v-model="fixedHeader" />
    </div>
    <div class="starter-setting-section">
      <div class="starter-setting-label">{{ $t('theme.appearance.watermark') }}</div>
      <TSwitch v-model="watermarkVisible" />
    </div>
    <div class="starter-setting-divider">{{ $t('theme.appearance.accessibility') }}</div>
    <div class="starter-setting-section">
      <div class="starter-setting-label">{{ $t('theme.appearance.grayscale') }}</div>
      <TSwitch v-model="grayscale" />
    </div>
    <div class="starter-setting-section">
      <div class="starter-setting-label">{{ $t('theme.appearance.colourWeakness') }}</div>
      <TSwitch v-model="colourWeakness" />
    </div>
  </TDrawer>
</template>

<style scoped>
.starter-setting-section {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}
.starter-setting-divider {
  margin: 8px 0 16px;
  padding-top: 12px;
  border-top: 1px solid var(--td-border-level-1-color);
  color: var(--td-text-color-secondary);
  font: var(--td-font-body-small);
}
.starter-setting-label {
  flex: 0 0 88px;
  color: var(--td-text-color-primary);
  font-size: 14px;
  line-height: 22px;
}
.starter-setting-section :deep(.t-select) {
  flex: 1 1 auto;
  min-width: 0;
}
</style>