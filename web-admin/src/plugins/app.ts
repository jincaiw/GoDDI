import type { App } from 'vue';
import { DialogPlugin } from 'tdesign-vue-next';
import { $t } from '@/locales';

export function setupAppErrorHandle(app: App) {
  app.config.errorHandler = (err, vm, info) => {
    // eslint-disable-next-line no-console
    console.error(err, vm, info);
  };
}

export function setupAppVersionNotification() {
  // Update check interval in milliseconds
  const UPDATE_CHECK_INTERVAL = 3 * 60 * 1000;

  const canAutoUpdateApp = import.meta.env.VITE_AUTOMATICALLY_DETECT_UPDATE === 'Y' && import.meta.env.PROD;
  if (!canAutoUpdateApp) return;

  let isShow = false;
  let updateInterval: ReturnType<typeof setInterval> | undefined;

  const checkForUpdates = async () => {
    if (isShow) return;

    const buildTime = await getHtmlBuildTime();

    // If failed to get build time or build time hasn't changed, no update is needed.
    if (!buildTime || buildTime === BUILD_TIME) {
      return;
    }

    isShow = true;

    // Show update notification (TDesign; no Naive UI provider needed).
    DialogPlugin.confirm({
      header: $t('system.updateTitle'),
      body: $t('system.updateContent'),
      confirmBtn: $t('system.updateConfirm'),
      cancelBtn: $t('system.updateCancel'),
      onConfirm: () => {
        isShow = false;
        location.reload();
      },
      onCancel: () => {
        isShow = false;
      },
      onClose: () => {
        isShow = false;
      }
    });
  };

  const startUpdateInterval = () => {
    if (updateInterval) {
      clearInterval(updateInterval);
    }
    updateInterval = setInterval(checkForUpdates, UPDATE_CHECK_INTERVAL);
  };

  // If updates should be checked, set up the visibility change listener and start the update interval
  if (!isShow && document.visibilityState === 'visible') {
    // Check for updates when the document is visible
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') {
        checkForUpdates();
        startUpdateInterval();
      }
    });

    // Start the update interval
    startUpdateInterval();
  }
}

async function getHtmlBuildTime(): Promise<string | null> {
  const baseUrl = import.meta.env.VITE_BASE_URL || '/';

  try {
    const res = await fetch(`${baseUrl}index.html?time=${Date.now()}`);

    if (!res.ok) {
      return null;
    }

    const html = await res.text();
    const match = html.match(/<meta name="buildTime" content="(.*)">/);
    return match?.[1] || null;
  } catch (error) {
    window.console.error('getHtmlBuildTime error:', error);
    return null;
  }
}
