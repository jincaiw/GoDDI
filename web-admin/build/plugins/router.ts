import type { RouteMeta } from 'vue-router';
import ElegantVueRouter from '@elegant-router/vue/vite';
import type { RouteKey } from '@elegant-router/types';

/**
 * Per-route meta config for the Goddi console.
 * Keys are Elegant Router route names (derived from the views directory).
 * icon/order drive the sidebar; permission drives the Goddi RBAC guard
 * (src/router/guard/permission.ts); hideInMenu for detail pages.
 */
const ROUTE_META: Partial<Record<RouteKey, Partial<RouteMeta>>> = {
      "dashboard": {
            "icon": "mdi:monitor-dashboard",
            "order": 1
      },
      "dns": {
            "icon": "mdi:dns",
            "order": 2
      },
      "dns_zones": {
            "icon": "mdi:dns",
            "order": 1,
            "permission": {
                  "resource": "dns",
                  "action": "read"
            }
      },
      "dns_zone-detail": {
            "permission": {
                  "resource": "dns",
                  "action": "read"
            },
            "hideInMenu": true
      },
      "dns_forwarders": {
            "icon": "mdi:swap-horizontal",
            "order": 2,
            "permission": {
                  "resource": "dns",
                  "action": "read"
            }
      },
      "dns_security": {
            "icon": "mdi:shield-check",
            "order": 3,
            "permission": {
                  "resource": "dns",
                  "action": "read"
            }
      },
      "dns_cache": {
            "icon": "mdi:database-clock",
            "order": 4,
            "permission": {
                  "resource": "dns",
                  "action": "read"
            }
      },
      "tools": {
            "icon": "mdi:wrench",
            "order": 3
      },
      "tools_client": {
            "icon": "mdi:console-network",
            "order": 1,
            "permission": {
                  "resource": "dns",
                  "action": "read"
            }
      },
      "dhcp": {
            "icon": "mdi:lan",
            "order": 4
      },
      "dhcp_scopes": {
            "icon": "mdi:map",
            "order": 1,
            "permission": {
                  "resource": "dhcp",
                  "action": "read"
            }
      },
      "dhcp_leases": {
            "icon": "mdi:license",
            "order": 2,
            "permission": {
                  "resource": "dhcp",
                  "action": "read"
            }
      },
      "dhcp_reservations": {
            "icon": "mdi:bookmark",
            "order": 3,
            "permission": {
                  "resource": "dhcp",
                  "action": "read"
            }
      },
      "dhcp_options": {
            "icon": "mdi:tune",
            "order": 4,
            "permission": {
                  "resource": "dhcp",
                  "action": "read"
            }
      },
      "ipam": {
            "icon": "mdi:ip-network",
            "order": 5
      },
      "ipam_spaces": {
            "icon": "mdi:earth",
            "order": 1,
            "permission": {
                  "resource": "ipam",
                  "action": "read"
            }
      },
      "ipam_subnets": {
            "icon": "mdi:subnet",
            "order": 2,
            "permission": {
                  "resource": "ipam",
                  "action": "read"
            }
      },
      "ipam_addresses": {
            "icon": "mdi:ip",
            "order": 3,
            "permission": {
                  "resource": "ipam",
                  "action": "read"
            }
      },
      "admin": {
            "icon": "mdi:shield-account",
            "order": 6
      },
      "admin_users": {
            "icon": "mdi:account-multiple",
            "order": 1,
            "permission": {
                  "resource": "user",
                  "action": "read"
            }
      },
      "admin_roles": {
            "icon": "mdi:account-key",
            "order": 2,
            "permission": {
                  "resource": "role",
                  "action": "read"
            }
      },
      "admin_groups": {
            "icon": "mdi:account-group",
            "order": 3,
            "permission": {
                  "resource": "group",
                  "action": "read"
            }
      },
      "admin_tokens": {
            "icon": "mdi:key-variant",
            "order": 4,
            "permission": {
                  "resource": "token",
                  "action": "read"
            }
      },
      "admin_sessions": {
            "icon": "mdi:monitor-lock",
            "order": 5,
            "permission": {
                  "resource": "user",
                  "action": "read"
            }
      },
      "logs": {
            "icon": "mdi:file-document-multiple",
            "order": 7
      },
      "logs_audit": {
            "icon": "mdi:clipboard-text-search",
            "order": 1,
            "permission": {
                  "resource": "audit",
                  "action": "read"
            }
      },
      "logs_dns": {
            "icon": "mdi:dns-search",
            "order": 2,
            "permission": {
                  "resource": "dns",
                  "action": "read"
            }
      },
      "logs_dhcp": {
            "icon": "mdi:lan-connect",
            "order": 3,
            "permission": {
                  "resource": "dhcp",
                  "action": "read"
            }
      },
      "settings": {
            "icon": "mdi:cog",
            "order": 8,
            "permission": {
                  "resource": "settings",
                  "action": "read"
            }
      },
      "settings_backup": {
            "icon": "mdi:backup-restore",
            "order": 1,
            "permission": {
                  "resource": "backup",
                  "action": "read"
            }
      }
};

export function setupElegantRouter() {
  return ElegantVueRouter({
    layouts: {
      base: 'src/layouts/base-layout/index.vue',
      blank: 'src/layouts/blank-layout/index.vue'
    },
    routePathTransformer(routeName, routePath) {
      const key = routeName as RouteKey;

      if (key === 'login') {
        const modules: UnionKey.LoginModule[] = ['pwd-login', 'code-login', 'register', 'reset-pwd', 'bind-wechat'];

        const moduleReg = modules.join('|');

        return `/login/:module(${moduleReg})?`;
      }

      return routePath;
    },
    onRouteMetaGen(routeName) {
      const key = routeName as RouteKey;

      const constantRoutes: RouteKey[] = ['login', '403', '404', '500'];

      const meta: Partial<RouteMeta> = {
        title: key,
        i18nKey: `route.${key}` as App.I18n.I18nKey,
        ...ROUTE_META[key]
      };

      if (constantRoutes.includes(key)) {
        meta.constant = true;
      }

      return meta;
    }
  });
}
