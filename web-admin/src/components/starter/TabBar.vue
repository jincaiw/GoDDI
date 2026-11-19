<script setup lang="ts">
import { computed } from 'vue';
import { useThemeStore } from '@/store/modules/theme';
import { useTabStore } from '@/store/modules/tab';

defineOptions({ name: 'StarterTabBar' });

/**
 * Route tab bar (starter "useTagTabs" element). The tab store already keeps
 * tabs, the fixed home tab and i18n sync; this component only renders them and
 * wires close/switch, and hides itself when the theme switch is off.
 */
const themeStore = useThemeStore();
const tabStore = useTabStore();

const visible = computed(() => themeStore.tab?.visible === true);
const tabs = computed(() => tabStore.tabs);
const activeId = computed(() => tabStore.activeTabId);

function isFixed(id: string) {
  return tabStore.isTabRetain(id);
}

async function onSwitch(id: string) {
  if (id === activeId.value) return;
  const tab = tabs.value.find((item: App.Global.Tab) => item.id === id);
  if (tab) await tabStore.switchRouteByTab(tab);
}

async function onClose(id: string) {
  await tabStore.removeTab(id);
}
</script>

<template>
  <div v-if="visible" class="td-tab-bar" role="tablist" :aria-label="$t('theme.appearance.useTagTabs')">
    <button
      v-for="tab in tabs"
      :key="tab.id"
      type="button"
      role="tab"
      :aria-selected="tab.id === activeId"
      class="td-tab"
      :class="{ 'td-tab--active': tab.id === activeId }"
      @click="onSwitch(tab.id)"
    >
      <span class="td-tab__label">{{ tab.label }}</span>
      <span
        v-if="!isFixed(tab.id)"
        class="td-tab__close"
        role="button"
        :aria-label="$t('common.close')"
        tabindex="0"
        @click.stop="onClose(tab.id)"
        @keydown.enter.stop="onClose(tab.id)"
      >
        <TIcon name="close" />
      </span>
    </button>
  </div>
</template>