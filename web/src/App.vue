<template>
  <n-config-provider :theme="appStore.darkMode ? darkTheme : null" :theme-overrides="themeOverrides" :locale="naiveLocale" :date-locale="naiveDateLocale">
    <n-message-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <n-loading-bar-provider>
            <router-view />
          </n-loading-bar-provider>
        </n-notification-provider>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, watchEffect } from 'vue'
import { darkTheme, zhCN, dateZhCN, enUS, dateEnUS } from 'naive-ui'
import { useAppStore } from '@/stores/app'
import { appleTheme } from '@/theme'

const appStore = useAppStore()
const themeOverrides = computed(() => appleTheme(appStore.darkMode))
watchEffect(() => {
  document.documentElement.dataset.theme = appStore.darkMode ? 'dark' : 'light'
  document.documentElement.lang = appStore.locale
})

const naiveLocale = computed(() => appStore.locale === 'zh-CN' ? zhCN : enUS)
const naiveDateLocale = computed(() => appStore.locale === 'zh-CN' ? dateZhCN : dateEnUS)
</script>

<style>
:root {
  --app-page-background: #f5f5f7;
  --app-surface: #fff;
  --app-text: #1d1d1f;
  --app-muted: #6e6e73;
  --app-border: #e5e5ea;
  color-scheme: light;
  font-family: 'Noto Sans SC', -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Helvetica Neue', sans-serif;
  -webkit-font-smoothing: antialiased;
}

:root[data-theme='dark'] {
  --app-page-background: #1c1c1e;
  --app-surface: #2c2c2e;
  --app-text: #ffffff;
  --app-muted: #a1a1a6;
  --app-border: #38383a;
  color-scheme: dark;
}

/* Only the app's own provider gets the full-viewport height. The discrete
   API (router/guards.ts) teleports an empty .n-config-provider directly to
   <body>; without this scoping it would add a full viewport of blank space
   below every page. */
#app > .n-config-provider {
  min-height: 100vh;
}

.n-card {
  border-radius: 10px;
  box-shadow: 0 2px 6px rgb(0 0 0 / 2%);
}

.filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 16px;
}

.n-data-table {
  border-radius: 10px;
  overflow-x: auto;
}

.n-modal, .n-dialog {
  max-width: calc(100vw - 32px);
  box-sizing: border-box;
}

.n-modal { margin: 24px auto; }
.n-modal .n-card__content { min-width: 0; }
.n-form-item-label { font-weight: 500; }
.n-data-table-th { white-space: nowrap; }
.n-button, .n-input, .n-base-selection { transition-duration: 160ms; }
:focus-visible { outline: 3px solid #78b5ff; outline-offset: 3px; }

@media (max-width: 640px) {
  .n-data-table-table { min-width: 640px; }
  .n-form-item.n-form-item--left-labelled {
    grid-template-areas: "label" "blank" "feedback";
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: auto auto auto;
  }
  .n-form-item .n-form-item-label,
  .n-form-item.n-form-item--left-labelled .n-form-item-label {
    display: flex;
    width: auto !important;
    min-height: 0;
    justify-content: flex-start;
    text-align: left;
    padding: 0 0 8px;
  }
  .n-modal .n-input-number { width: 100%; }
  .n-card-header { flex-wrap: wrap; gap: 12px; }
  .n-card-header__extra { margin-left: 0 !important; max-width: 100%; }
  .n-card > .n-card__content { padding-left: 16px; padding-right: 16px; }
  .n-space .n-input, .n-space .n-base-selection { max-width: calc(100vw - 72px); }
}

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { animation-duration: 0.01ms !important; transition-duration: 0.01ms !important; }
}

html,
body,
#app {
  min-width: 320px;
  min-height: 100%;
  margin: 0;
  background: var(--app-page-background);
  color: var(--app-text);
}
</style>
