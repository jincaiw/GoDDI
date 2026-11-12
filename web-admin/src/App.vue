<script setup lang="ts">
import { computed } from 'vue';
import { ConfigProvider as TConfigProvider } from 'tdesign-vue-next';
import type { GlobalConfigProvider } from 'tdesign-vue-next';
import zhConfig from 'tdesign-vue-next/es/locale/zh_CN';
import enConfig from 'tdesign-vue-next/es/locale/en_US';
import { useAppStore } from './store/modules/app';

defineOptions({
  name: 'App'
});

const appStore = useAppStore();

// TDesign is the only UI library in use; the locale modules are declared
// `readonly` while GlobalConfigProvider uses mutable tuples, so the cast only
// widens the type with no runtime change.
const tdesignLocale = computed<GlobalConfigProvider>(
  () => (appStore.locale === 'en-US' ? enConfig : zhConfig) as unknown as GlobalConfigProvider
);
</script>

<template>
  <TConfigProvider :global-config="tdesignLocale">
    <RouterView class="bg-layout" />
  </TConfigProvider>
</template>

<style scoped></style>
