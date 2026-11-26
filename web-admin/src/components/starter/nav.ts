/** Frozen GoDDI navigation contract for the starter-style shell. */
export interface NavItem {
  key: string;
  labelKey: string;
  icon: string;
  permission?: { resource: string; action: string };
  children?: NavItem[];
}

/** Keep the visible menu aligned with route guards and API permissions. */
export function accessibleNav(
  items: NavItem[],
  can: (resource: string, action: string) => boolean
): NavItem[] {
  return items.flatMap(item => {
    if (item.permission && !can(item.permission.resource, item.permission.action)) return [];
    if (!item.children) return [item];
    const children = accessibleNav(item.children, can);
    return children.length ? [{ ...item, children }] : [];
  });
}

/** Minimal route shape used by the NAV/route permission consistency check. */
interface NavPermissionRoute {
  name?: string | symbol | null;
  meta?: { permission?: { resource: string; action: string } | null } | null;
}

/**
 * Detect drift between the visible menu and the router's permission meta.
 * A mismatch either hides a reachable page or shows one the guard will reject.
 * Only keys present in both NAV and the router are compared.
 */
export function findNavPermissionMismatches(items: NavItem[], routes: NavPermissionRoute[]): string[] {
  const routePermissions = new Map<string, { resource: string; action: string }>();

  for (const route of routes) {
    const permission = route.meta?.permission;
    if (route.name && permission) routePermissions.set(String(route.name), permission);
  }

  const mismatches: string[] = [];
  const walk = (navItems: NavItem[]) => {
    for (const item of navItems) {
      const routePermission = routePermissions.get(item.key);
      if (routePermission && item.permission) {
        const { resource, action } = item.permission;
        if (routePermission.resource !== resource || routePermission.action !== action) {
          mismatches.push(
            `${item.key}: nav=${resource}:${action}, route=${routePermission.resource}:${routePermission.action}`
          );
        }
      }
      if (item.children) walk(item.children);
    }
  };

  walk(items);
  return mismatches;
}

export const NAV: NavItem[] = [
  { key: 'dashboard', labelKey: 'route.dashboard', icon: 'dashboard' },
  {
    key: 'dns',
    labelKey: 'route.dns',
    icon: 'server',
    children: [
      { key: 'dns_zones', labelKey: 'route.dns_zones', icon: 'internet', permission: { resource: 'dns', action: 'read' } },
      { key: 'dns_forwarders', labelKey: 'route.dns_forwarders', icon: 'swap', permission: { resource: 'dns', action: 'read' } },
      { key: 'dns_cache', labelKey: 'route.dns_cache', icon: 'data', permission: { resource: 'dns', action: 'read' } },
      { key: 'dns_allowed', labelKey: 'route.dns_allowed', icon: 'check-circle', permission: { resource: 'dns', action: 'read' } },
      { key: 'dns_blocked', labelKey: 'route.dns_blocked', icon: 'error-circle', permission: { resource: 'dns', action: 'read' } },
      { key: 'dns_security', labelKey: 'route.dns_security', icon: 'secured', permission: { resource: 'dns', action: 'read' } },
      { key: 'dns_dnssec', labelKey: 'route.dns_dnssec', icon: 'lock-on', permission: { resource: 'dns', action: 'read' } },
      { key: 'dns_apps', labelKey: 'route.dns_apps', icon: 'app', permission: { resource: 'dns', action: 'read' } }
    ]
  },
  {
    key: 'dhcp',
    labelKey: 'route.dhcp',
    icon: 'layers',
    children: [
      { key: 'dhcp_scopes', labelKey: 'route.dhcp_scopes', icon: 'map-location', permission: { resource: 'dhcp', action: 'read' } },
      { key: 'dhcp_leases', labelKey: 'route.dhcp_leases', icon: 'time', permission: { resource: 'dhcp', action: 'read' } },
      { key: 'dhcp_reservations', labelKey: 'route.dhcp_reservations', icon: 'bookmark', permission: { resource: 'dhcp', action: 'read' } },
      { key: 'dhcp_options', labelKey: 'route.dhcp_options', icon: 'setting', permission: { resource: 'dhcp', action: 'read' } },
      { key: 'dhcp_ha', labelKey: 'route.dhcp_ha', icon: 'connection', permission: { resource: 'dhcp', action: 'read' } }
    ]
  },
  {
    key: 'ipam',
    labelKey: 'route.ipam',
    icon: 'root-list',
    children: [
      { key: 'ipam_spaces', labelKey: 'route.ipam_spaces', icon: 'earth', permission: { resource: 'ipam', action: 'read' } },
      { key: 'ipam_subnets', labelKey: 'route.ipam_subnets', icon: 'fork', permission: { resource: 'ipam', action: 'read' } },
      { key: 'ipam_addresses', labelKey: 'route.ipam_addresses', icon: 'location', permission: { resource: 'ipam', action: 'read' } },
      { key: 'ipam_retirement-archive', labelKey: 'route.ipam_retirement-archive', icon: 'archive', permission: { resource: 'ipam', action: 'read' } }
    ]
  },
  {
    key: 'logs',
    labelKey: 'route.logs',
    icon: 'file-paste',
    children: [
      { key: 'logs_dns', labelKey: 'route.logs_dns', icon: 'chart-bar', permission: { resource: 'dns', action: 'read' } },
      { key: 'logs_dhcp', labelKey: 'route.logs_dhcp', icon: 'file-1', permission: { resource: 'dhcp', action: 'read' } },
      { key: 'tools_client', labelKey: 'route.tools_client', icon: 'terminal-rectangle', permission: { resource: 'dns', action: 'read' } },
      { key: 'logs_audit', labelKey: 'route.logs_audit', icon: 'file-search', permission: { resource: 'audit', action: 'read' } }
    ]
  },
  {
    key: 'admin',
    labelKey: 'route.admin',
    icon: 'usergroup',
    children: [
      { key: 'admin_users', labelKey: 'route.admin_users', icon: 'user', permission: { resource: 'user', action: 'read' } },
      { key: 'admin_groups', labelKey: 'route.admin_groups', icon: 'usergroup', permission: { resource: 'group', action: 'read' } },
      { key: 'admin_roles', labelKey: 'route.admin_roles', icon: 'user-safety', permission: { resource: 'role', action: 'read' } },
      { key: 'admin_tokens', labelKey: 'route.admin_tokens', icon: 'lock-on', permission: { resource: 'token', action: 'read' } },
      { key: 'admin_sessions', labelKey: 'route.admin_sessions', icon: 'desktop', permission: { resource: 'user', action: 'read' } }
    ]
  },
  {
    key: 'settings',
    labelKey: 'route.settings',
    icon: 'setting',
    children: [
      { key: 'settings_system', labelKey: 'route.settings_system', icon: 'system-setting', permission: { resource: 'settings', action: 'read' } },
      { key: 'settings_config-versions', labelKey: 'route.settings_config-versions', icon: 'history', permission: { resource: 'settings', action: 'read' } },
      { key: 'settings_backup', labelKey: 'route.settings_backup', icon: 'cloud-upload', permission: { resource: 'backup', action: 'read' } }
    ]
  }
];

/** Locate the displayed group even when a bookmarked URL has a different prefix. */
export function navParent(key: string) {
  return NAV.find(item => item.children?.some(child => child.key === key));
}

/** route name -> path, kept in sync with src/router/elegant/routes.ts */
export const PATHS: Record<string, string> = {
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
  'ipam_retirement-archive': '/ipam/retirement-archive',
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