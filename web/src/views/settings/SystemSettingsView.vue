<template>
  <div>
    <page-header :title="t('settings.title')" />

    <n-spin :show="loading">
      <n-card>
        <n-form label-placement="left" label-width="200px">
          <n-form-item v-for="setting in settings" :key="setting.key" :label="settingLabel(setting.key)">
            <n-switch v-if="setting.type === 'bool'" v-model:value="setting.value" :checked-value="'true'" :unchecked-value="'false'" />
            <n-input-number v-else-if="setting.type === 'int'" v-model:value="numericValues[setting.key]" :min="0" style="max-width: 500px;" />
            <n-input v-else v-model:value="setting.value" :type="isLongValue(String(setting.value)) ? 'textarea' : 'text'" :rows="3" style="max-width: 500px;" />
            <n-button type="primary" size="small" style="margin-left: 8px;" :loading="savingKeys[setting.key]" @click="handleSave(setting)">{{ t('common.save') }}</n-button>
            <template #feedback>
              <span style="color: var(--n-text-color-3); font-size: 12px;">{{ settingDesc(setting) }}</span>
            </template>
          </n-form-item>
        </n-form>
      </n-card>
    </n-spin>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import { listSystemSettings, updateSystemSetting, type SystemSetting } from '@/api/settings'

const { t } = useI18n()
const message = useMessage()

// Localized label/description for a setting key; falls back to the raw key /
// backend description for unknown keys.
function settingLabel(key: string): string {
  const localized = t(`settings.items.${key}.label`)
  return localized !== `settings.items.${key}.label` ? localized : key
}

function settingDesc(setting: SystemSetting): string {
  const localized = t(`settings.items.${setting.key}.desc`)
  return localized !== `settings.items.${setting.key}.desc` ? localized : setting.description
}

const loading = ref(false)
const settings = ref<SystemSetting[]>([])
const savingKeys = reactive<Record<string, boolean>>({})

// Fallback list: if the backend ever returns settings without a `type` field
// (older versions, ad-hoc inserts) we still want a sensible default. The
// backend now sends `type` for every well-known key, so this only fires for
// unknown keys.
const FALLBACK_BOOL_KEYS = ['server_dark_mode', 'dns_enable_logging', 'dhcp_enable_failover']
const FALLBACK_NUMBER_KEYS = ['dns_default_ttl', 'dns_cache_max_entries', 'dhcp_default_lease_time', 'dhcp_max_lease_time']

function effectiveType(setting: SystemSetting): 'bool' | 'int' | 'string' {
  if (setting.type === 'bool' || setting.type === 'int' || setting.type === 'string') {
    return setting.type
  }
  if (FALLBACK_BOOL_KEYS.includes(setting.key) || setting.value === 'true' || setting.value === 'false') {
    return 'bool'
  }
  if (FALLBACK_NUMBER_KEYS.includes(setting.key)) {
    return 'int'
  }
  if (typeof setting.value === 'string' && /^\d+$/.test(setting.value)) {
    return 'int'
  }
  return 'string'
}

function isBoolSetting(setting: SystemSetting): boolean {
  return effectiveType(setting) === 'bool'
}

function isNumberSetting(setting: SystemSetting): boolean {
  return effectiveType(setting) === 'int'
}

function isLongValue(value: string): boolean {
  return !!value && value.length > 100
}

// Track numeric values separately for n-input-number binding
const numericValues = reactive<Record<string, number>>({})

async function loadData() {
  loading.value = true
  try {
    const result = await listSystemSettings()
    settings.value = result.data
    // Initialize numeric values
    for (const s of result.data) {
      if (isNumberSetting(s)) {
        numericValues[s.key] = Number(s.value)
      }
    }
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

async function handleSave(setting: SystemSetting) {
  savingKeys[setting.key] = true
  try {
    const valueToSave = isNumberSetting(setting) ? String(numericValues[setting.key] ?? setting.value) : setting.value
    await updateSystemSetting(setting.key, valueToSave)
    message.success(t('settings.updateSuccess'))
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { savingKeys[setting.key] = false }
}

onMounted(loadData)
</script>
