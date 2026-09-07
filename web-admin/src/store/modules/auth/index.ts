import { computed, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { defineStore } from 'pinia';
import { useLoading } from '@sa/hooks';
import { fetchGetUserInfo, fetchLogin, fetchLogout } from '@/service/api';
import type { UserInfo } from '@/service/api/goddi/auth';
import { $t } from '@/locales';
import { SetupStoreId } from '@/enum';
import { useRouteStore } from '../route';
import { useTabStore } from '../tab';
import { clearAuthStorage, getToken, setAuthStorage } from './shared';

type GoddiPermission = { resource: string; action: string };

/**
 * Goddi auth store.
 *
 * Keeps the template's store surface (token/userInfo/isLogin/login/resetStore/
 * initUserInfo) so the layout, guards and builtin login modules keep working,
 * but the internals are backed by the Goddi API: JWT in sessionStorage, JWT
 * expiry-aware isLogin, and the resource+action RBAC model from /auth/me.
 */
export const useAuthStore = defineStore(SetupStoreId.Auth, () => {
  const route = useRoute();
  const router = useRouter();
  const routeStore = useRouteStore();
  const tabStore = useTabStore();
  const { loading: loginLoading, startLoading, endLoading } = useLoading();

  const token = ref(getToken());

  const userInfo = reactive<Api.Auth.UserInfo>({
    userId: '',
    userName: '',
    roles: [],
    buttons: []
  });

  /** raw /auth/me payload, kept for permission checks */
  const rawUserInfo = ref<UserInfo | null>(null);

  /** is super role in static route */
  const isStaticSuper = computed(() => {
    const { VITE_AUTH_ROUTE_MODE, VITE_STATIC_SUPER_ROLE } = import.meta.env;

    return VITE_AUTH_ROUTE_MODE === 'static' && userInfo.roles.includes(VITE_STATIC_SUPER_ROLE);
  });

  /** JWT expiry-aware login state (mirrors legacy isTokenExpired) */
  function isTokenExpired(t: string): boolean {
    if (!t) return true;
    try {
      const payload = t.split('.')[1];
      if (!payload) return true;
      const base64 = payload.replace(/-/g, '+').replace(/_/g, '/');
      const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);
      const decoded = JSON.parse(atob(padded));
      if (!decoded.exp) return false;
      return decoded.exp * 1000 < Date.now();
    } catch {
      return true;
    }
  }

  const isLogin = computed(() => Boolean(token.value) && !isTokenExpired(token.value));

  /** Goddi RBAC: resource + action permission check */
  function hasPermission(resource: string, action: string): boolean {
    const perms = rawUserInfo.value?.permissions as GoddiPermission[] | undefined;
    if (!perms) return false;
    return perms.some(p => p.resource === resource && p.action === action);
  }

  function hasAnyPermission(checks: Array<{ resource: string; action: string }>): boolean {
    return checks.some(({ resource, action }) => hasPermission(resource, action));
  }

  /** Reset auth store */
  async function resetStore() {
    clearAuthStorage();
    token.value = '';
    rawUserInfo.value = null;
    userInfo.userId = '';
    userInfo.userName = '';
    userInfo.roles = [];
    userInfo.buttons = [];

    if (!route.meta.constant) {
      await router.push('/login');
    }

    tabStore.cacheTabs();
    routeStore.resetStore();
  }

  /**
   * Login
   *
   * @param userName User name
   * @param password Password
   * @param [totpCode] Optional TOTP verification code
   * @throws Error with the backend message on failure
   */
  async function login(userName: string, password: string, totpCode?: string) {
    startLoading();

    try {
      const { data: loginData, error } = await fetchLogin(userName, password, totpCode);

      if (error || !loginData) {
        throw error ?? new Error($t('page.login.common.loginSuccess') ? 'Login failed' : 'Login failed');
      }

      // 1. store tokens (sessionStorage)
      setAuthStorage(loginData.token, loginData.csrf_token);
      token.value = loginData.token;

      // 2. get user info
      const pass = await getUserInfoInstance();

      if (!pass) {
        clearAuthStorage();
        token.value = '';
        throw new Error('Failed to load user info');
      }

      // 3. redirect (sanitized, mirrors legacy LoginView logic)
      checkTabClear();
      const redirect = (route.query?.redirect as string) || '/';
      const safeRedirect = redirect.startsWith('/') && !redirect.startsWith('//') && !redirect.includes('\\') ? redirect : '/';
      await router.push(safeRedirect);

      window.$notification?.success({
        title: $t('page.login.common.loginSuccess'),
        content: $t('page.login.common.welcomeBack', { userName: userInfo.userName }),
        duration: 4500
      });
    } finally {
      endLoading();
    }
  }

  async function getUserInfoInstance() {
    const { data, error } = await fetchGetUserInfo();

    if (error || !data) {
      return false;
    }

    rawUserInfo.value = data;
    userInfo.userId = data.id;
    userInfo.userName = data.username;
    userInfo.roles = data.roles.map(r => r.name);
    userInfo.buttons = [];

    return true;
  }

  /** Record the user ID of the previous login session, used to clear tabs on user switch */
  function recordUserId() {
    if (!userInfo.userId) {
      return;
    }
    sessionStorage.setItem('lastLoginUserId', userInfo.userId);
  }

  function checkTabClear(): boolean {
    if (!userInfo.userId) {
      return false;
    }

    const lastLoginUserId = sessionStorage.getItem('lastLoginUserId');

    if (!lastLoginUserId || lastLoginUserId !== userInfo.userId) {
      tabStore.clearTabs();
      sessionStorage.removeItem('lastLoginUserId');
      return true;
    }

    sessionStorage.removeItem('lastLoginUserId');
    return false;
  }

  async function logout() {
    try {
      await fetchLogout();
    } catch {
      // ignore logout API errors (session may already be gone)
    } finally {
      recordUserId();
      await resetStore();
    }
  }

  async function initUserInfo() {
    const maybeToken = getToken();

    if (maybeToken) {
      token.value = maybeToken;
      const pass = await getUserInfoInstance();

      if (!pass) {
        await resetStore();
      }
    }
  }

  return {
    token,
    userInfo,
    isStaticSuper,
    isLogin,
    loginLoading,
    hasPermission,
    hasAnyPermission,
    resetStore,
    login,
    logout,
    initUserInfo
  };
});
