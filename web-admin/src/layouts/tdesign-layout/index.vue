<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useAuthStore } from '@/store/modules/auth';
import { $t } from '@/locales';
import NodeModeBadge from '@/components/NodeModeBadge.vue';

defineOptions({ name: 'TDesignLayout' });

/**
 * TDesign Vue Next Starter-style shell.
 *
 * This is the target console layout for the staged migration (see
 * docs/tdesign-migration.md). It renders the existing 38 page components
 * inside <RouterView />, so every page stays reachable while each page is
 * migrated from Naive UI to TDesign one at a time.
 *
 * Starter layers kept: layouts (header/aside/content/footer + menu),
 * router-driven active menu, store-backed user state, and a single
 * <RouterView /> content region. Unused soybean tab/theme-drawer modules are
 * intentionally not rendered here; pages do not depend on their context.
 */

interface NavItem {
  key: string;
  labelKey: string;
  icon: string;
  children?: NavItem[];
}

const NAV: NavItem[] = [
  { key: 'dashboard', labelKey: 'route.dashboard', icon: 'dashboard' },
  {
    key: 'dns',
    labelKey: 'route.dns',
    icon: 'server',
    children: [
      { key: 'dns_zones', labelKey: 'route.dns_zones', icon: 'internet' },
      { key: 'dns_forwarders', labelKey: 'route.dns_forwarders', icon: 'swap' },
      { key: 'dns_allowed', labelKey: 'route.dns_allowed', icon: 'check-circle' },
      { key: 'dns_blocked', labelKey: 'route.dns_blocked', icon: 'error-circle' },
      { key: 'dns_security', labelKey: 'route.dns_security', icon: 'secured' },
      { key: 'dns_dnssec', labelKey: 'route.dns_dnssec', icon: 'lock-on' },
      { key: 'dns_cache', labelKey: 'route.dns_cache', icon: 'data' },
      { key: 'dns_apps', labelKey: 'route.dns_apps', icon: 'app' }
    ]
  },
  {
    key: 'dhcp',
    labelKey: 'route.dhcp',
    icon: 'layers',
    children: [
      { key: 'dhcp_scopes', labelKey: 'route.dhcp_scopes', icon: 'map-location' },
      { key: 'dhcp_leases', labelKey: 'route.dhcp_leases', icon: 'time' },
      { key: 'dhcp_reservations', labelKey: 'route.dhcp_reservations', icon: 'bookmark' },
      { key: 'dhcp_options', labelKey: 'route.dhcp_options', icon: 'setting' },
      { key: 'dhcp_ha', labelKey: 'route.dhcp_ha', icon: 'connection' }
    ]
  },
  {
    key: 'ipam',
    labelKey: 'route.ipam',
    icon: 'root-list',
    children: [
      { key: 'ipam_spaces', labelKey: 'route.ipam_spaces', icon: 'earth' },
      { key: 'ipam_subnets', labelKey: 'route.ipam_subnets', icon: 'fork' },
      { key: 'ipam_addresses', labelKey: 'route.ipam_addresses', icon: 'location' }
    ]
  },
  {
    key: 'tools',
    labelKey: 'route.tools',
    icon: 'tools',
    children: [{ key: 'tools_client', labelKey: 'route.tools_client', icon: 'terminal-rectangle' }]
  },
  {
    key: 'admin',
    labelKey: 'route.admin',
    icon: 'usergroup',
    children: [
      { key: 'admin_users', labelKey: 'route.admin_users', icon: 'user' },
      { key: 'admin_roles', labelKey: 'route.admin_roles', icon: 'user-safety' },
      { key: 'admin_groups', labelKey: 'route.admin_groups', icon: 'usergroup' },
      { key: 'admin_tokens', labelKey: 'route.admin_tokens', icon: 'lock-on' },
      { key: 'admin_sessions', labelKey: 'route.admin_sessions', icon: 'desktop' }
    ]
  },
  {
    key: 'logs',
    labelKey: 'route.logs',
    icon: 'file-paste',
    children: [
      { key: 'logs_audit', labelKey: 'route.logs_audit', icon: 'file-search' },
      { key: 'logs_dns', labelKey: 'route.logs_dns', icon: 'chart-bar' },
      { key: 'logs_dhcp', labelKey: 'route.logs_dhcp', icon: 'file-1' }
    ]
  },
  {
    key: 'settings',
    labelKey: 'route.settings',
    icon: 'setting',
    children: [
      { key: 'settings_system', labelKey: 'route.settings_system', icon: 'system-setting' },
      { key: 'settings_backup', labelKey: 'route.settings_backup', icon: 'cloud-upload' },
      { key: 'settings_config-versions', labelKey: 'route.settings_config-versions', icon: 'history' }
    ]
  }
  // STAGE 3 RESERVATION (dual-node / HA): a "cluster" nav group goes here once
  // internal/cluster coordination and B8.5 dual-host evidence exist. It is
  // deliberately not registered in the single-node phase: the backend
  // /api/v1/cluster endpoints return 501 today.
];

/** route name -> path, kept in sync with src/router/elegant/routes.ts */
const PATHS: Record<string, string> = {
  dashboard: '/dashboard',
  dns_zones: '/dns/zones',
  dns_forwarders: '/dns/forwarders',
  dns_allowed: '/dns/allowed',
  dns_blocked: '/dns/blocked',
  dns_security: '/dns/security',
  dns_dnssec: '/dns/dnssec',
  dns_cache: '/dns/cache',
  dns_apps: '/dns/apps',
  dhcp_scopes: '/dhcp/scopes',
  dhcp_leases: '/dhcp/leases',
  dhcp_reservations: '/dhcp/reservations',
  dhcp_options: '/dhcp/options',
  dhcp_ha: '/dhcp/ha',
  ipam_spaces: '/ipam/spaces',
  ipam_subnets: '/ipam/subnets',
  ipam_addresses: '/ipam/addresses',
  tools_client: '/tools/client',
  admin_users: '/admin/users',
  admin_roles: '/admin/roles',
  admin_groups: '/admin/groups',
  admin_tokens: '/admin/tokens',
  admin_sessions: '/admin/sessions',
  logs_audit: '/logs/audit',
  logs_dns: '/logs/dns',
  logs_dhcp: '/logs/dhcp',
  settings_system: '/settings/system',
  settings_backup: '/settings/backup',
  'settings_config-versions': '/settings/config-versions'
};

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();

const collapsed = ref(false);
const activeMenu = computed(() => String(route.name || 'dashboard'));
const expanded = ref<string[]>(['dns', 'dhcp', 'ipam', 'admin', 'logs', 'settings', 'tools']);

function go(key: string) {
  const path = PATHS[key];
  if (path) void router.push(path);
}

function handleMenuChange(value: string) {
  go(value);
}

async function handleLogout() {
  await authStore.resetStore();
  await router.push('/login');
}

const userName = computed(() => authStore.userInfo.userName || 'Admin');
</script>

<template>
  <TLayout class="td-shell">
    <TAside :width="collapsed ? '64px' : '232px'" class="td-aside">
      <div class="td-logo" @click="go('dashboard')">
        <TIcon name="logo-github-filled" size="26px" />
        <span v-if="!collapsed" class="td-logo-text">GoDDI Console</span>
      </div>
      <TMenu
        :value="activeMenu"
        :expanded="expanded"
        :collapsed="collapsed"
        theme="dark"
        @change="handleMenuChange"
        @expand="(v: string[]) => (expanded = v)"
      >
        <template v-for="item in NAV" :key="item.key">
          <TSubmenu v-if="item.children" :value="item.key">
            <template #icon><TIcon :name="item.icon" /></template>
            <template #title>{{ $t(item.labelKey as any) }}</template>
            <TMenuItem v-for="child in item.children" :key="child.key" :value="child.key">
              <template #icon><TIcon :name="child.icon" /></template>
              {{ $t(child.labelKey as any) }}
            </TMenuItem>
          </TSubmenu>
          <TMenuItem v-else :value="item.key">
            <template #icon><TIcon :name="item.icon" /></template>
            {{ $t(item.labelKey as any) }}
          </TMenuItem>
        </template>
      </TMenu>
    </TAside>

    <TLayout class="td-main">
      <THeader class="td-header">
        <TButton variant="text" shape="square" @click="collapsed = !collapsed">
          <TIcon :name="collapsed ? 'menu-unfold' : 'menu-fold'" size="20px" />
        </TButton>
        <div class="td-breadcrumb">{{ $t((route.meta.i18nKey as any) || 'route.dashboard') }}</div>
        <div class="td-header-right">
          <!--
 Stage 3 (dual-node) binds `mode` to a trusted cluster status.
               Single-node phase pins it to "single" and adds no HA state. 
-->
          <NodeModeBadge mode="single" />
          <TDropdown :options="[{ content: '退出登录', value: 'logout' }]" @click="handleLogout">
            <TButton variant="text">
              <TIcon name="user-circle" size="18px" />
              <span class="td-user">{{ userName }}</span>
            </TButton>
          </TDropdown>
        </div>
      </THeader>

      <TContent class="td-content">
        <RouterView />
      </TContent>

      <TFooter class="td-footer">GoDDI Console · Vue 3 + TypeScript + TDesign</TFooter>
    </TLayout>
  </TLayout>
</template>

<style scoped>
.td-shell {
  height: 100vh;
  width: 100%;
}
.td-aside {
  background: #001529;
  display: flex;
  flex-direction: column;
  /* The sider is a flex item; without flex-shrink:0 the full-width content
     pane squeezes it to ~0 even though TDesign sets width:232px inline. */
  flex: 0 0 auto;
  overflow: hidden;
  transition: width 0.2s;
}
.td-logo {
  height: 56px;
  flex: 0 0 56px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 18px;
  color: #fff;
  cursor: pointer;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  white-space: nowrap;
}
.td-logo-text {
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 0.5px;
}
.td-aside :deep(.t-menu) {
  border-right: none;
  flex: 1;
  overflow-y: auto;
}
.td-main {
  min-width: 0;
}
.td-header {
  height: 56px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
  background: #fff;
  border-bottom: 1px solid #e7e7e7;
}
.td-breadcrumb {
  font-size: 14px;
  color: #4b5563;
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
  overflow: auto;
  background: #f3f4f6;
}
.td-footer {
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  color: #9ca3af;
  background: #fff;
  border-top: 1px solid #e7e7e7;
}
</style>
