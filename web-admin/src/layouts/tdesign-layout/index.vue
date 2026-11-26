<script setup lang="ts">
import { $t } from '@/locales';
import { computed, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useRoute, useRouter } from 'vue-router';
import { useAppStore } from '@/store/modules/app';
import { useThemeStore } from '@/store/modules/theme';
import { navParent, PATHS } from '@/components/starter/nav';
import StarterHeader from '@/components/starter/Header.vue';
import StarterSideNav from '@/components/starter/SideNav.vue';
import StarterSettingDrawer from '@/components/starter/SettingDrawer.vue';
import StarterTabBar from '@/components/starter/TabBar.vue';

defineOptions({ name: 'TDesignLayout' });

const route = useRoute();
const router = useRouter();
const appStore = useAppStore();
const themeStore = useThemeStore();

/** Collapse state lives in the app store so the mobile watcher can force the sider closed. */
const { siderCollapse: collapsed, isMobile } = storeToRefs(appStore);

// Hidden detail routes (for example dns_zone-detail) declare the menu key
// that should stay highlighted; fall back to the route name / dashboard.
const activeMenu = computed(() => String(route.meta.activeMenu || route.name || 'dashboard'));
const layoutMode = computed(() => String(themeStore.layout?.mode || 'vertical'));
const footerVisible = computed(() => themeStore.footer?.visible !== false);
const fixedHeader = computed(() => Boolean(themeStore.fixedHeaderAndTab));
const isHorizontal = computed(() => layoutMode.value === 'horizontal');
const expanded = ref<string[]>([]);

// Route-level page cache (starter "keep-alive" behaviour).
//
// KeepAlive matches on the COMPONENT name, not the route name, so the include
// list must be derived from the resolved component of each cacheable route.
// Views declare `defineOptions({ name: 'DNSZonesPage' })`; route names are
// snake_case (`dns_zones`), which is why matching route names never worked.

watch(
  activeMenu,
  key => {
    const group = navParent(key)?.key;
    if (group && !expanded.value.includes(group)) expanded.value = [...expanded.value, group];
  },
  { immediate: true }
);

function closeDrawer() {
  if (isMobile.value) appStore.setSiderCollapse(true);
}

function go(key: string) {
  const path = PATHS[key];
  if (!path) return;
  closeDrawer();
  void router.push(path);
}
function handleMenuChange(value: string) { go(value); }
function toggleCollapse() { appStore.toggleSiderCollapse(); }
</script>

<template>
  <TLayout
    class="td-shell"
    :class="{ 'td-shell--mobile': isMobile, 'td-shell--drawer-open': isMobile && !collapsed }"
  >
    <StarterSideNav
      v-if="!isHorizontal"
      :active="activeMenu"
      :collapsed="collapsed"
      :expanded="expanded"
      @change="handleMenuChange"
      @expand="(v: string[]) => (expanded = v)"
      @home="go('dashboard')"
    />
    <!-- Scrim makes the expanded mobile sider an explicit, dismissible drawer. -->
    <div v-if="isMobile && !collapsed" class="td-sider-mask" aria-hidden="true" @click="closeDrawer" />
    <TLayout class="td-main">
      <StarterHeader
        :collapsed="collapsed"
        :horizontal="isHorizontal"
        :active="activeMenu"
        @toggle-collapse="toggleCollapse"
        @menu-change="handleMenuChange"
      />
      <StarterTabBar />
      <TContent class="td-content" :class="{ 'td-content--fixed-header': fixedHeader }">
        <RouterView />
      </TContent>
      <TFooter v-if="footerVisible" class="td-footer">{{ $t('system.footer') }}</TFooter>
    </TLayout>
    <StarterSettingDrawer />
  </TLayout>
</template>

<style scoped>
.td-shell {
  height: 100vh;
  width: 100%;
}
.td-aside {
  background: var(--shell-sidebar-bg);
  display: flex;
  flex-direction: column;
  /* The sider is a flex item; without flex-shrink:0 the full-width content
     pane squeezes it to ~0 even though TDesign sets width:232px inline. */
  flex: 0 0 auto;
  overflow: hidden;
  transition: width 0.2s;
}
.td-aside :deep(.t-menu) {
  border-right: none;
  flex: 1;
  overflow-y: auto;
}
.td-sider-mask {
  /* Only rendered in mobile drawer mode; sits below the sider, above content. */
  position: fixed;
  inset: 0;
  z-index: 1000;
  background: rgb(0 0 0 / 45%);
}
.td-main {
  min-width: 0;
}
.td-content--fixed-header {
  overflow: auto;
}
.td-header {
  height: 56px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
  background: var(--shell-header-bg);
  color: var(--shell-text);
  border-bottom: 1px solid var(--shell-border);
}
.td-top-menu {
  flex: 1;
  min-width: 0;
}
.td-header-horizontal {
  gap: 8px;
}
.td-breadcrumb {
  font-size: 14px;
  color: var(--shell-text);
}
.td-header-right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 12px;
}
.td-user {
  margin-left: 6px;
}
.td-content {
  padding: 16px;
  background: var(--shell-content-bg);
  overflow: auto;
}
.td-footer {
  padding: 0 16px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--shell-footer-bg);
  color: var(--shell-muted);
  font-size: 12px;
  border-top: 1px solid var(--shell-border);
}
@media (max-width: 640px) {
  .td-shell--mobile .td-aside {
    position: fixed;
    top: 0;
    bottom: 0;
    left: 0;
    z-index: 1001;
    box-shadow: var(--td-shadow-2);
  }
  .td-shell--mobile .td-main {
    width: 100%;
  }
  .td-content {
    padding: 12px;
  }
}
</style>