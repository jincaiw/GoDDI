<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useAuthStore } from '@/store/modules/auth';
import { $t, getLocale } from '@/locales';
import { useAppStore } from '@/store/modules/app';
import { useThemeStore } from '@/store/modules/theme';
import { accessibleNav, NAV, PATHS } from '@/components/starter/nav';
import { getConfigReplicationStatus } from '@/service/api/goddi/ha';
import { getHAStatus } from '@/service/api/goddi/ha';
import NodeModeBadge from '@/components/NodeModeBadge.vue';
import StarterLanguageSwitcher from '@/components/starter/LanguageSwitcher.vue';

defineOptions({ name: 'StarterHeader' });
const { collapsed, horizontal, active } = defineProps<{ collapsed: boolean; horizontal: boolean; active: string }>();
const emit = defineEmits<{ (e: 'toggleCollapse'): void; (e: 'menuChange', value: string): void }>();
const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const appStore = useAppStore();
const themeStore = useThemeStore();
interface BreadcrumbCrumb {
  key: string;
  label: string;
  to?: { name: string };
  current?: boolean;
}

function crumbLabel(meta: { i18nKey?: unknown; title?: unknown } | undefined, fallback: string): string {
  const i18nKey = meta?.i18nKey;
  if (i18nKey) return String($t(i18nKey as any));
  return String(meta?.title || fallback);
}

// The devflow theme leaves breadcrumb.visible false, so it stays hidden by default.
const breadcrumbVisible = computed(() => themeStore.header?.breadcrumb?.visible === true);
const breadcrumbLabel = computed(() => (getLocale() === 'zh-CN' ? '面包屑导航' : 'Breadcrumb'));

/**
 * Build the chain from the matched records. Unnamed layout wrappers and
 * constant pages (login/errors) are skipped; the current leaf stays as text.
 */
const breadcrumbs = computed<BreadcrumbCrumb[]>(() => {
  const matched = route.matched.filter(record => {
    const name = String(record.name ?? '');
    return Boolean(name) && name !== 'root' && !record.meta?.constant;
  });

  const items: BreadcrumbCrumb[] = matched.map((record, index) => {
    const key = String(record.name);
    const current = index === matched.length - 1;
    return {
      key,
      label: crumbLabel(record.meta, key),
      ...(!current && !record.path.includes(':') ? { to: { name: key } } : {}),
      ...(current ? { current: true } : {})
    };
  });

  // Hidden detail routes (e.g. dns_zone-detail) declare the list route that
  // should remain visible as their parent in the hierarchy.
  const activeMenuName = route.meta?.activeMenu ? String(route.meta.activeMenu) : '';
  if (activeMenuName && !items.some(item => item.key === activeMenuName)) {
    const parent = router.getRoutes().find(record => record.name === activeMenuName);
    if (parent) {
      items.splice(Math.max(items.length - 1, 0), 0, {
        key: activeMenuName,
        label: crumbLabel(parent.meta, activeMenuName),
        to: { name: activeMenuName }
      });
    }
  }

  return items;
});

const navItems = computed(() => accessibleNav(NAV, (resource, action) => authStore.hasPermission(resource, action)));
const quickSearch = ref('');
const searchItems = computed(() => {
  const flatten = (items: typeof NAV): Array<{ key: string; label: string }> =>
    items.flatMap(item => [
      ...(item.children ? flatten(item.children) : []),
      ...(!item.children ? [{ key: item.key, label: String($t(item.labelKey as any)) }] : [])
    ]);
  return flatten(navItems.value);
});

/**
 * Starter-style quick search: results appear as you type instead of requiring
 * Enter, and an empty result set is reported rather than silently ignored.
 */
const searchOpen = ref(false);
const searchResults = computed(() => {
  const query = quickSearch.value.trim().toLowerCase();
  if (!query) return [];
  return searchItems.value
    .filter(item => item.label.toLowerCase().includes(query) || item.key.toLowerCase().includes(query))
    .slice(0, 8);
});

function goToResult(key: string) {
  const path = PATHS[key];
  if (!path) return;
  searchOpen.value = false;
  quickSearch.value = '';
  void router.push(path);
}

function handleQuickSearch() {
  const first = searchResults.value[0];
  if (first) goToResult(first.key);
}

function onSearchFocus() {
  searchOpen.value = Boolean(quickSearch.value.trim());
}

function onSearchBlur() {
  // Delay so a click on a result registers before the panel unmounts.
  window.setTimeout(() => {
    searchOpen.value = false;
  }, 150);
}

// --- Notification center (real signals only) ---
// Decision: the starter header shows a notification bell, but GoDDI has no
// notification API. Rather than a decorative empty bell, surface the two
// health signals the console already owns: HA state and config replication.
interface HeaderNotice {
  key: string;
  label: string;
  theme: 'warning' | 'error';
  to: string;
}

const notices = ref<HeaderNotice[]>([]);

async function refreshNotices() {
  const found: HeaderNotice[] = [];
  const [ha, repl] = await Promise.allSettled([getHAStatus(), getConfigReplicationStatus()]);

  if (ha.status === 'fulfilled') {
    const state = String((ha.value as { state?: string })?.state || '');
    if (state && !['primary', 'standalone', 'active', 'single'].includes(state)) {
      found.push({ key: 'ha', label: $t('haStatus.state') + '：' + state, theme: 'warning', to: '/dhcp/ha' });
    }
  }
  if (repl.status === 'fulfilled') {
    const r = repl.value as { healthy?: boolean; last_error?: string };
    if (r && r.healthy === false) {
      found.push({
        key: 'repl',
        label: $t('haStatus.replAlertTitle') + (r.last_error ? '：' + r.last_error : ''),
        theme: 'error',
        to: '/settings/config-versions'
      });
    }
  }
  notices.value = found;
}

let noticeTimer: number | undefined;
onMounted(() => {
  void refreshNotices();
  noticeTimer = window.setInterval(() => void refreshNotices(), 30000);
});
onUnmounted(() => {
  if (noticeTimer !== undefined) window.clearInterval(noticeTimer);
});

function openNotice(notice: HeaderNotice) {
  void router.push(notice.to);
}

// --- Fullscreen (native browser capability) ---
const isFullscreen = ref(false);
function syncFullscreen() {
  isFullscreen.value = Boolean(document.fullscreenElement);
}
async function toggleFullscreen() {
  if (document.fullscreenElement) {
    await document.exitFullscreen();
  } else {
    await document.documentElement.requestFullscreen();
  }
  syncFullscreen();
}
onMounted(() => document.addEventListener('fullscreenchange', syncFullscreen));
onUnmounted(() => document.removeEventListener('fullscreenchange', syncFullscreen));

const userMenuItems = computed(() => [
  { content: $t('sessions.title'), value: 'sessions' },
  { content: $t('common.logout'), value: 'logout' }
]);

async function handleUserMenu(value: unknown) {
  const action = String(value);
  if (action === 'sessions') {
    await router.push('/admin/sessions');
    return;
  }
  if (action === 'logout') await handleLogout();
}

const userName = computed(() => authStore.userInfo.userName || 'Admin');
async function handleLogout() {
  await authStore.resetStore();
  await router.push('/login');
}
function menuChange(value: string) { emit('menuChange', value); }
</script>

<template>
  <THeader class="td-header" :class="{ 'td-header-horizontal': horizontal }">
    <TButton v-if="!horizontal" variant="text" shape="square" :aria-label="collapsed ? $t('icon.expand') : $t('icon.collapse')" @click="emit('toggleCollapse')">
      <TIcon :name="collapsed ? 'menu-unfold' : 'menu-fold'" size="20px" />
    </TButton>
    <div class="td-quick-search-wrap">
      <TInput
        v-model="quickSearch"
        class="td-quick-search"
        :placeholder="$t('common.search')"
        :aria-label="$t('common.search')"
        clearable
        @enter="handleQuickSearch"
        @focus="onSearchFocus"
        @blur="onSearchBlur"
        @change="onSearchFocus"
      >
        <template #prefix-icon><TIcon name="search" /></template>
      </TInput>
      <div v-if="searchOpen" class="td-search-panel" role="listbox" :aria-label="$t('common.search')">
        <button
          v-for="item in searchResults"
          :key="item.key"
          type="button"
          role="option"
          :aria-selected="false"
          class="td-search-item"
          @mousedown.prevent="goToResult(item.key)"
        >
          <TIcon name="search" />
          <span>{{ item.label }}</span>
        </button>
        <p v-if="!searchResults.length" class="td-search-empty" role="status">
          {{ $t('common.searchNoResult') }}
        </p>
      </div>
    </div>
    <nav v-if="breadcrumbVisible && breadcrumbs.length" class="td-breadcrumb" :aria-label="breadcrumbLabel">
      <TBreadcrumb>
        <TBreadcrumbItem
          v-for="crumb in breadcrumbs"
          :key="crumb.key"
          :to="crumb.to"
          :aria-current="crumb.current ? 'page' : undefined"
        >
          {{ crumb.label }}
        </TBreadcrumbItem>
      </TBreadcrumb>
    </nav>
    <!--
 Top menu: one entry per top-level group, in navigation order. Mixing
         TMenuItem and TSubmenu via two filtered v-for loops made TDesign render
         each submenu title as a second clickable item; build the list once.
-->
    <TMenu v-if="horizontal" :value="active" theme="light" class="td-top-menu" @change="menuChange">
      <template v-for="item in navItems" :key="item.key">
        <TSubmenu v-if="item.children && item.children.length" :value="item.key">
          <template #title>{{ $t(item.labelKey as any) }}</template>
          <TMenuItem v-for="child in item.children" :key="child.key" :value="child.key">
            {{ $t(child.labelKey as any) }}
          </TMenuItem>
        </TSubmenu>
        <TMenuItem v-else :value="item.key">
          {{ $t(item.labelKey as any) }}
        </TMenuItem>
      </template>
    </TMenu>
    <div class="td-header-right">
      <NodeModeBadge />
      <StarterLanguageSwitcher />
      <TPopup trigger="click" placement="bottom-right" :overlay-inner-style="{ padding: '0' }">
        <template #content>
          <div class="td-notice-panel">
            <div class="td-notice-header">{{ $t('common.notifications') }}</div>
            <div v-if="!notices.length" class="td-notice-empty">{{ $t('common.noNotifications') }}</div>
            <button
              v-for="notice in notices"
              :key="notice.key"
              type="button"
              class="td-notice-item"
              :class="'td-notice-item--' + notice.theme"
              @click="openNotice(notice)"
            >
              <TIcon :name="notice.theme === 'error' ? 'error-circle' : 'error-triangle'" />
              <span>{{ notice.label }}</span>
            </button>
          </div>
        </template>
        <TBadge :count="notices.length" :offset="[-2, 2]">
          <TButton variant="text" shape="square" :aria-label="$t('common.notifications')">
            <TIcon name="notification" size="18px" />
          </TButton>
        </TBadge>
      </TPopup>
      <TButton
        variant="text"
        shape="square"
        :aria-label="isFullscreen ? $t('icon.fullscreenExit') : $t('icon.fullscreen')"
        @click="toggleFullscreen"
      >
        <TIcon :name="isFullscreen ? 'fullscreen-exit' : 'fullscreen'" size="18px" />
      </TButton>
      <TButton variant="text" shape="square" :aria-label="$t('theme.themeDrawerTitle')" @click="appStore.openThemeDrawer()">
        <TIcon name="setting" size="18px" />
      </TButton>
      <TDropdown :options="userMenuItems" @click="handleUserMenu">
        <TButton variant="text" :aria-label="$t('common.logout')">
          <TIcon name="user-circle" size="18px" />
          <span class="td-user">{{ userName }}</span>
        </TButton>
      </TDropdown>
    </div>
  </THeader>
</template>

<style scoped>
.td-quick-search {
  width: 240px;
  max-width: 32vw;
}
.td-notice-panel {
  width: 320px;
  max-height: 360px;
  overflow-y: auto;
  padding: 8px;
}
.td-notice-header {
  padding: 4px 8px 8px;
  font: var(--td-font-title-small);
  color: var(--td-text-color-primary);
}
.td-notice-empty {
  padding: 24px 8px;
  text-align: center;
  color: var(--td-text-color-placeholder);
  font-size: 12px;
}
.td-notice-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  width: 100%;
  padding: 8px;
  border: 0;
  border-radius: var(--td-radius-default);
  background: transparent;
  color: var(--td-text-color-primary);
  font: var(--td-font-body-small);
  text-align: left;
  cursor: pointer;
}
.td-notice-item:hover { background: var(--td-bg-color-container-hover); }
.td-notice-item--warning .t-icon { color: var(--td-warning-color); }
.td-notice-item--error .t-icon { color: var(--td-error-color); }
.td-breadcrumb {
  min-width: 0;
  font-size: 14px;
  color: var(--shell-text);
}
.td-breadcrumb :deep(.t-breadcrumb) {
  max-width: 100%;
  overflow: hidden;
}
@media (max-width: 640px) {
  /* Nonessential text yields to the icon-only controls on small screens. */
  .td-breadcrumb,
  .td-user,
  .td-quick-search {
    display: none;
  }
  .td-header {
    gap: 8px;
    padding: 0 10px;
  }
  .td-header-right {
    gap: 4px;
  }
}
</style>
