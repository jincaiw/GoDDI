import type { Router } from 'vue-router';
import { MessagePlugin, NotifyPlugin } from 'tdesign-vue-next';

/**
 * A failed request on page A can leave an error toast with a long duration.
 * Navigating to page B should not show that stale error: it describes a
 * context the operator is no longer looking at. Close TDesign messages and
 * notifications after a completed navigation. Navigations to 403/login are
 * skipped: the warning that explains the redirect is raised by a guard and
 * must remain visible.
 */
export function createToastCleanupGuard(router: Router) {
  router.afterEach((to, from) => {
    // The first navigation has no previous route; there is nothing stale yet.
    if (!from.name) return;

    const routeName = String(to.name ?? '');
    if (routeName === '403' || routeName === 'login') return;

    MessagePlugin.closeAll();
    NotifyPlugin.closeAll();
  });
}
