import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/LoginView.vue'),
    meta: { requiresAuth: false },
  },
  {
    path: '/',
    component: () => import('@/components/AppLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      {
        path: '',
        name: 'Dashboard',
        component: () => import('@/views/DashboardView.vue'),
      },
      // DNS
      {
        path: 'dns/zones',
        name: 'DNSZones',
        component: () => import('@/views/dns/ZonesView.vue'),
        meta: { permission: { resource: 'dns', action: 'read' } },
      },
      {
        path: 'dns/zones/:id',
        name: 'DNSZoneDetail',
        component: () => import('@/views/dns/ZoneDetailView.vue'),
        meta: { permission: { resource: 'dns', action: 'read' } },
      },
      {
        path: 'dns/forwarders',
        name: 'DNSForwarders',
        component: () => import('@/views/dns/ForwardersView.vue'),
        meta: { permission: { resource: 'dns', action: 'read' } },
      },
      {
        path: 'dns/security',
        name: 'DNSSecurity',
        component: () => import('@/views/dns/SecurityView.vue'),
        meta: { permission: { resource: 'dns', action: 'read' } },
      },
      {
        path: 'dns/cache',
        name: 'DNSCache',
        component: () => import('@/views/dns/CacheView.vue'),
        meta: { permission: { resource: 'dns', action: 'read' } },
      },
      // Tools
      {
        path: 'tools/client',
        name: 'DNSClient',
        component: () => import('@/views/dns/ClientView.vue'),
        meta: { permission: { resource: 'dns', action: 'read' } },
      },
      // DHCP
      {
        path: 'dhcp/scopes',
        name: 'DHCPScopes',
        component: () => import('@/views/dhcp/ScopesView.vue'),
        meta: { permission: { resource: 'dhcp', action: 'read' } },
      },
      {
        path: 'dhcp/leases',
        name: 'DHCPLeases',
        component: () => import('@/views/dhcp/LeasesView.vue'),
        meta: { permission: { resource: 'dhcp', action: 'read' } },
      },
      {
        path: 'dhcp/reservations',
        name: 'DHCPReservations',
        component: () => import('@/views/dhcp/ReservationsView.vue'),
        meta: { permission: { resource: 'dhcp', action: 'read' } },
      },
      {
        path: 'dhcp/options',
        name: 'DHCPOptions',
        component: () => import('@/views/dhcp/OptionsView.vue'),
        meta: { permission: { resource: 'dhcp', action: 'read' } },
      },
      // IPAM
      {
        path: 'ipam/spaces',
        name: 'IPAMSpaces',
        component: () => import('@/views/ipam/SpacesView.vue'),
        meta: { permission: { resource: 'ipam', action: 'read' } },
      },
      {
        path: 'ipam/subnets',
        name: 'IPAMSubnets',
        component: () => import('@/views/ipam/SubnetsView.vue'),
        meta: { permission: { resource: 'ipam', action: 'read' } },
      },
      {
        path: 'ipam/addresses',
        name: 'IPAMAddresses',
        component: () => import('@/views/ipam/AddressesView.vue'),
        meta: { permission: { resource: 'ipam', action: 'read' } },
      },
      // Admin
      {
        path: 'admin/users',
        name: 'AdminUsers',
        component: () => import('@/views/admin/UsersView.vue'),
        meta: { permission: { resource: 'user', action: 'read' } },
      },
      {
        path: 'admin/roles',
        name: 'AdminRoles',
        component: () => import('@/views/admin/RolesView.vue'),
        meta: { permission: { resource: 'role', action: 'read' } },
      },
      {
        path: 'admin/groups',
        name: 'AdminGroups',
        component: () => import('@/views/admin/GroupsView.vue'),
        meta: { permission: { resource: 'group', action: 'read' } },
      },
      {
        path: 'admin/tokens',
        name: 'AdminTokens',
        component: () => import('@/views/admin/TokensView.vue'),
        meta: { permission: { resource: 'token', action: 'read' } },
      },
      {
        path: 'admin/sessions',
        name: 'AdminSessions',
        component: () => import('@/views/admin/SessionsView.vue'),
        meta: { permission: { resource: 'user', action: 'read' } },
      },
      // Logs
      {
        path: 'logs/audit',
        name: 'AuditLogs',
        component: () => import('@/views/logs/AuditLogsView.vue'),
        meta: { permission: { resource: 'audit', action: 'read' } },
      },
      {
        path: 'logs/dns',
        name: 'DNSQueryLogs',
        component: () => import('@/views/logs/QueryLogsView.vue'),
        meta: { permission: { resource: 'dns', action: 'read' } },
      },
      {
        path: 'logs/dhcp',
        name: 'DHCPLogs',
        component: () => import('@/views/logs/DHCPLogsView.vue'),
        meta: { permission: { resource: 'dhcp', action: 'read' } },
      },
      // Settings
      {
        path: 'settings',
        name: 'SystemSettings',
        component: () => import('@/views/settings/SystemSettingsView.vue'),
        meta: { permission: { resource: 'settings', action: 'read' } },
      },
      {
        path: 'settings/backup',
        name: 'Backup',
        component: () => import('@/views/settings/BackupView.vue'),
        meta: { permission: { resource: 'backup', action: 'read' } },
      },
    ],
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/',
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
