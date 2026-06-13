import { defineStore } from 'pinia'
import { ref } from 'vue'
import i18n from '@/i18n'

export const useAppStore = defineStore('app', () => {
  const sidebarCollapsed = ref(false)
  const darkMode = ref(localStorage.getItem('dark_mode') === 'true')
  const locale = ref(localStorage.getItem('locale') || 'zh-CN')

  function toggleSidebar() {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  function setDarkMode(value: boolean) {
    darkMode.value = value
    localStorage.setItem('dark_mode', String(value))
  }

  function toggleDarkMode() {
    setDarkMode(!darkMode.value)
  }

  function setLocale(value: string) {
    locale.value = value
    localStorage.setItem('locale', value)
    i18n.global.locale.value = value as 'zh-CN' | 'en-US'
  }

  return {
    sidebarCollapsed,
    darkMode,
    locale,
    toggleSidebar,
    setDarkMode,
    toggleDarkMode,
    setLocale,
  }
})
