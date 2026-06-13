<template>
  <n-config-provider :theme="appStore.darkMode ? darkTheme : null" :locale="naiveLocale" :date-locale="naiveDateLocale">
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
import { computed } from 'vue'
import { darkTheme, zhCN, dateZhCN, enUS, dateEnUS } from 'naive-ui'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()

const naiveLocale = computed(() => appStore.locale === 'zh-CN' ? zhCN : enUS)
const naiveDateLocale = computed(() => appStore.locale === 'zh-CN' ? dateZhCN : dateEnUS)
</script>

<style>
:root {
  --app-page-background: #f5f7fa;
}

.n-config-provider {
  min-height: 100vh;
}

.n-card {
  border-radius: 12px;
}

.n-data-table {
  border-radius: 12px;
  overflow: hidden;
}

@media (prefers-color-scheme: dark) {
  :root {
    --app-page-background: #101014;
  }
}

html,
body,
#app {
  min-width: 320px;
  min-height: 100%;
  margin: 0;
}
</style>
