<template>
  <n-layout has-sider class="app-shell">
    <n-layout-sider
      v-if="!isMobile"
      bordered
      collapse-mode="width"
      :collapsed-width="64"
      :width="232"
      :collapsed="collapsed"
      show-trigger
      @collapse="collapsed = true"
      @expand="collapsed = false"
      :native-scrollbar="false"
      class="desktop-sider"
    >
      <div class="logo" :class="{ 'logo-collapsed': collapsed }">
        <span class="logo-icon">G</span>
        <span v-if="!collapsed" class="logo-text">GoDDI</span>
      </div>
      <n-menu
        :collapsed="collapsed"
        :collapsed-width="64"
        :collapsed-icon-size="22"
        :options="menuOptions"
        :value="activeKey"
        @update:value="handleMenuSelect"
        :render-label="(renderMenuLabel as any)"
      />
    </n-layout-sider>
    <n-drawer v-model:show="mobileMenuOpen" placement="left" :width="280">
      <n-drawer-content body-content-style="padding: 0;" closable>
        <div class="logo mobile-logo">
          <span class="logo-icon">G</span>
          <span class="logo-text">GoDDI</span>
        </div>
        <n-menu
          :options="menuOptions"
          :value="activeKey"
          @update:value="handleMenuSelect"
          :render-label="(renderMenuLabel as any)"
        />
      </n-drawer-content>
    </n-drawer>
    <n-layout class="main-layout">
      <n-layout-header bordered class="app-header">
        <div class="header-left">
          <n-button v-if="isMobile" quaternary circle aria-label="Open navigation" @click="mobileMenuOpen = true">
            <template #icon><n-icon><menu-outline /></n-icon></template>
          </n-button>
          <n-breadcrumb>
            <n-breadcrumb-item
              v-for="item in breadcrumbs"
              :key="item.path"
              class="breadcrumb-item"
              @click="router.resolve(item.path).matched.some(record => record.path === item.path) && router.push(item.path)"
            >
              {{ item.label }}
            </n-breadcrumb-item>
          </n-breadcrumb>
        </div>
        <div class="header-actions">
          <n-dropdown :options="languageOptions" @select="handleLanguageSelect">
            <n-button quaternary circle :aria-label="t('common.language')">
              <template #icon><n-icon><language-outline /></n-icon></template>
            </n-button>
          </n-dropdown>
          <n-switch size="small" :aria-label="t('common.appearance')" :value="isDark" @update:value="toggleDark">
            <template #checked>
              <n-icon><sunny-outline /></n-icon>
            </template>
            <template #unchecked>
              <n-icon><moon-outline /></n-icon>
            </template>
          </n-switch>
          <n-dropdown :options="userDropdownOptions" @select="handleUserAction">
            <n-button quaternary>
              <template #icon>
                <n-icon><person-outline /></n-icon>
              </template>
              <span v-if="!isMobile">{{ displayName }}</span>
            </n-button>
          </n-dropdown>
        </div>
      </n-layout-header>
      <n-layout-content
        :content-style="isMobile ? 'padding: 24px 16px;' : 'padding: 32px;'"
        :native-scrollbar="false"
        class="app-content"
        :class="{ 'app-content-dark': isDark }"
      >
        <div class="content-container">
          <router-view />
        </div>
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NIcon } from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import {
  ServerOutline,
  GlobeOutline,
  SwapHorizontalOutline,
  ShieldCheckmarkOutline,
  SearchOutline,
  TerminalOutline,
  DesktopOutline,
  DocumentTextOutline,
  KeyOutline,
  PeopleOutline,
  SettingsOutline,
  CloudDownloadOutline,
  LogOutOutline,
  PersonOutline,
  MoonOutline,
  SunnyOutline,
  HomeOutline,
  GridOutline,
  LayersOutline,
  ListOutline,
  MenuOutline,
  LanguageOutline,
} from '@vicons/ionicons5'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const collapsed = computed({
  get: () => appStore.sidebarCollapsed,
  set: (value: boolean) => { appStore.sidebarCollapsed = value },
})
const isMobile = ref(false)
const mobileMenuOpen = ref(false)
const isDark = computed(() => appStore.darkMode)

function updateViewport() {
  isMobile.value = window.innerWidth < 900
  if (!isMobile.value) mobileMenuOpen.value = false
}

onMounted(() => {
  updateViewport()
  window.addEventListener('resize', updateViewport)
})
onBeforeUnmount(() => window.removeEventListener('resize', updateViewport))

function toggleDark() {
  appStore.toggleDarkMode()
}

const displayName = computed(() => authStore.user?.display_name || authStore.user?.username || '')

const activeKey = computed(() => {
  const path = route.path
  const keys = menuOptionList
    .flatMap((item) => [item, ...(item.children ?? [])])
    .map((item) => String(item.key))
    .filter((key) => key.startsWith('/'))
    .sort((a, b) => b.length - a.length)
  for (const key of keys) {
    if (path.startsWith(key)) return key
  }
  return '/'
})

const breadcrumbs = computed(() => {
  const items = [{ label: 'GoDDI', path: '/' }]
  const path = route.path
  if (path.startsWith('/dns/client')) {
    items.push({ label: t('nav.tools'), path: '/dns/client' })
    items.push({ label: t('nav.dnsClient'), path: '/dns/client' })
  } else if (path.startsWith('/dns')) {
    items.push({ label: t('nav.dns'), path: '/dns' })
    if (path.includes('/zones')) items.push({ label: t('nav.dnsZones'), path: '/dns/zones' })
    if (path.includes('/forwarders')) items.push({ label: t('nav.dnsForwarders'), path: '/dns/forwarders' })
    if (path.includes('/security')) items.push({ label: t('nav.dnsSecurity'), path: '/dns/security' })
    if (path.includes('/cache')) items.push({ label: t('nav.dnsCache'), path: '/dns/cache' })
    if (path.includes('/client')) items.push({ label: t('nav.dnsClient'), path: '/dns/client' })
  } else if (path.startsWith('/dhcp')) {
    items.push({ label: t('nav.dhcp'), path: '/dhcp' })
    if (path.includes('/scopes')) items.push({ label: t('nav.dhcpScopes'), path: '/dhcp/scopes' })
    if (path.includes('/leases')) items.push({ label: t('nav.dhcpLeases'), path: '/dhcp/leases' })
    if (path.includes('/reservations')) items.push({ label: t('nav.dhcpReservations'), path: '/dhcp/reservations' })
    if (path.includes('/options')) items.push({ label: t('nav.dhcpOptions'), path: '/dhcp/options' })
  } else if (path.startsWith('/ipam')) {
    items.push({ label: t('nav.ipam'), path: '/ipam' })
    if (path.includes('/spaces')) items.push({ label: t('nav.ipamSpaces'), path: '/ipam/spaces' })
    if (path.includes('/subnets')) items.push({ label: t('nav.ipamSubnets'), path: '/ipam/subnets' })
    if (path.includes('/addresses')) items.push({ label: t('nav.ipamAddresses'), path: '/ipam/addresses' })
  } else if (path.startsWith('/admin')) {
    items.push({ label: t('nav.administration'), path: '/admin' })
    if (path.includes('/users')) items.push({ label: t('nav.adminUsers'), path: '/admin/users' })
    if (path.includes('/roles')) items.push({ label: t('nav.adminRoles'), path: '/admin/roles' })
    if (path.includes('/groups')) items.push({ label: t('nav.adminGroups'), path: '/admin/groups' })
    if (path.includes('/tokens')) items.push({ label: t('nav.adminTokens'), path: '/admin/tokens' })
  } else if (path.startsWith('/logs')) {
    items.push({ label: t('nav.logs'), path: '/logs' })
    if (path.includes('/audit')) items.push({ label: t('nav.logsAudit'), path: '/logs/audit' })
    if (path.includes('/dns')) items.push({ label: t('nav.logsDns'), path: '/logs/dns' })
    if (path.includes('/dhcp')) items.push({ label: t('nav.logsDhcp'), path: '/logs/dhcp' })
  } else if (path.startsWith('/settings')) {
    items.push({ label: t('nav.administration'), path: '/admin' })
    if (path.includes('/backup')) items.push({ label: t('nav.settingsBackup'), path: '/settings/backup' })
    else items.push({ label: t('nav.settings'), path: '/settings' })
  }
  return items
})

function renderIcon(icon: typeof HomeOutline) {
  return () => h(NIcon, null, { default: () => h(icon) })
}

function renderMenuLabel(option: Record<string, unknown>): string {
  const label = option.label
  if (typeof label === 'function') {
    return label()
  }
  return (label as string) || ''
}

const menuOptionList: MenuOption[] = [
  { key: '/', label: () => t('nav.dashboard'), icon: renderIcon(HomeOutline) },
  {
    key: '/dns',
    label: () => t('nav.dns'),
    icon: renderIcon(GlobeOutline),
    children: [
      { key: '/dns/zones', label: () => t('nav.dnsZones'), icon: renderIcon(LayersOutline) },
      { key: '/dns/forwarders', label: () => t('nav.dnsForwarders'), icon: renderIcon(SwapHorizontalOutline) },
      { key: '/dns/security', label: () => t('nav.dnsSecurity'), icon: renderIcon(ShieldCheckmarkOutline) },
      { key: '/dns/cache', label: () => t('nav.dnsCache'), icon: renderIcon(ServerOutline) },
    ],
  },
  {
    key: '/dhcp',
    label: () => t('nav.dhcp'),
    icon: renderIcon(DesktopOutline),
    children: [
      { key: '/dhcp/scopes', label: () => t('nav.dhcpScopes'), icon: renderIcon(GridOutline) },
      { key: '/dhcp/leases', label: () => t('nav.dhcpLeases'), icon: renderIcon(DocumentTextOutline) },
      { key: '/dhcp/reservations', label: () => t('nav.dhcpReservations'), icon: renderIcon(ListOutline) },
      { key: '/dhcp/options', label: () => t('nav.dhcpOptions'), icon: renderIcon(SettingsOutline) },
    ],
  },
  {
    key: '/ipam',
    label: () => t('nav.ipam'),
    icon: renderIcon(ServerOutline),
    children: [
      { key: '/ipam/spaces', label: () => t('nav.ipamSpaces'), icon: renderIcon(LayersOutline) },
      { key: '/ipam/subnets', label: () => t('nav.ipamSubnets'), icon: renderIcon(GridOutline) },
      { key: '/ipam/addresses', label: () => t('nav.ipamAddresses'), icon: renderIcon(ListOutline) },
    ],
  },
  {
    key: '/tools',
    label: () => t('nav.tools'),
    icon: renderIcon(TerminalOutline),
    children: [
      { key: '/dns/client', label: () => t('nav.dnsClient'), icon: renderIcon(TerminalOutline) },
    ],
  },
  {
    key: '/logs',
    label: () => t('nav.logs'),
    icon: renderIcon(SearchOutline),
    children: [
      { key: '/logs/audit', label: () => t('nav.logsAudit'), icon: renderIcon(DocumentTextOutline) },
      { key: '/logs/dns', label: () => t('nav.logsDns'), icon: renderIcon(GlobeOutline) },
      { key: '/logs/dhcp', label: () => t('nav.logsDhcp'), icon: renderIcon(DesktopOutline) },
    ],
  },
  {
    key: '/admin',
    label: () => t('nav.administration'),
    icon: renderIcon(PeopleOutline),
    children: [
      { key: '/admin/users', label: () => t('nav.adminUsers'), icon: renderIcon(PersonOutline) },
      { key: '/admin/groups', label: () => t('nav.adminGroups'), icon: renderIcon(PeopleOutline) },
      { key: '/admin/roles', label: () => t('nav.adminRoles'), icon: renderIcon(KeyOutline) },
      { key: '/admin/tokens', label: () => t('nav.adminTokens'), icon: renderIcon(KeyOutline) },
      { key: '/settings', label: () => t('nav.settings'), icon: renderIcon(SettingsOutline) },
      { key: '/settings/backup', label: () => t('nav.settingsBackup'), icon: renderIcon(CloudDownloadOutline) },
    ],
  },
]

type PermissionCheck = { resource: string; action: string }

const menuPermissions: Record<string, PermissionCheck> = {
  '/dns': { resource: 'dns', action: 'read' },
  '/dhcp': { resource: 'dhcp', action: 'read' },
  '/ipam': { resource: 'ipam', action: 'read' },
  '/admin/users': { resource: 'user', action: 'read' },
  '/admin/roles': { resource: 'role', action: 'read' },
  '/admin/groups': { resource: 'group', action: 'read' },
  '/admin/tokens': { resource: 'token', action: 'read' },
  '/logs/audit': { resource: 'audit', action: 'read' },
  '/logs/dns': { resource: 'dns', action: 'read' },
  '/logs/dhcp': { resource: 'dhcp', action: 'read' },
  '/settings': { resource: 'settings', action: 'read' },
  '/settings/backup': { resource: 'backup', action: 'read' },
}

function canAccess(key: string) {
  const permission = menuPermissions[key]
  return !permission || authStore.hasPermission(permission.resource, permission.action)
}

const menuOptions = computed<MenuOption[]>(() => menuOptionList.flatMap((item): MenuOption[] => {
  const itemKey = String(item.key)
  if (!canAccess(itemKey)) return []
  if (item.children) {
    const children = item.children.filter((child) => canAccess(String(child.key)))
    if (children.length === 0) return []
    return [{ ...item, children }]
  }
  return canAccess(itemKey) ? [item] : []
}))

function handleMenuSelect(key: string) {
  mobileMenuOpen.value = false
  router.push(key)
}

const languageOptions = computed(() => [
  { label: 'English', key: 'en-US', disabled: appStore.locale === 'en-US' },
  { label: '简体中文', key: 'zh-CN', disabled: appStore.locale === 'zh-CN' },
])

function handleLanguageSelect(key: string) {
  appStore.setLocale(key)
}

const userDropdownOptions = [
  { label: () => t('auth.logout'), key: 'logout', icon: renderIcon(LogOutOutline) },
]

async function handleUserAction(key: string) {
  if (key === 'logout') {
    await authStore.logout()
    router.push('/login')
  }
}
</script>

<style scoped>
.logo {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  height: 88px;
  padding: 0 20px;
  gap: 12px;
  overflow: hidden;
  white-space: nowrap;
}

.app-shell,
.desktop-sider {
  height: 100vh;
  height: 100dvh;
}

.desktop-sider :deep(.n-menu) { padding: 8px; }
.desktop-sider :deep(.n-menu-item-content) { padding-left: 16px !important; }
.desktop-sider :deep(.n-submenu-children .n-menu-item-content) { padding-left: 28px !important; }
.desktop-sider :deep(.n-menu-item-content__icon) { font-size: 19px !important; }

.main-layout {
  min-width: 0;
}

.app-header {
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  background: color-mix(in srgb, var(--app-surface) 86%, transparent);
  backdrop-filter: blur(24px) saturate(160%);
}

.header-left,
.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.breadcrumb-item {
  cursor: pointer;
}

.app-content {
  height: calc(100vh - 64px);
  height: calc(100dvh - 64px);
  background: var(--app-page-background);
}

.app-content-dark {
  --app-page-background: #1c1c1e;
}

.content-container {
  width: 100%;
  max-width: 1680px;
  margin: 0 auto;
}

.mobile-logo {
  justify-content: flex-start;
}

.logo-collapsed {
  padding: 0;
  justify-content: center;
}

.logo-icon {
  font-size: 22px;
  font-weight: 600;
  color: #fff;
  background: linear-gradient(155deg, #49a3ff, #007aff);
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  border-radius: 10px;
  box-shadow: 0 3px 8px #007aff20;
  flex-shrink: 0;
}

.logo-text {
  font-size: 22px;
  font-weight: 600;
  letter-spacing: -0.6px;
  color: var(--app-text);
}

@media (max-width: 899px) {
  .app-header {
    padding: 0 12px;
  }

  .header-actions {
    gap: 4px;
  }
  .header-left .n-breadcrumb { overflow: hidden; white-space: nowrap; }
}
</style>
