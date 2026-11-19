<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import { usePermission } from '@/composables/usePermission';
import {
  listSystemSettings,
  updateSystemSetting,
  type SystemSetting
} from '@/service/api/goddi/settings';

defineOptions({ name: 'SystemSettingsPage' });

const { t } = useI18n();
const perm = usePermission();

const loading = ref(false);
const settings = ref<SystemSetting[]>([]);
const savingKeys = reactive<Record<string, boolean>>({});
const numericValues = reactive<Record<string, number>>({});

// Localized label/description for a setting key; falls back to the raw key /
// backend description for unknown keys.
function settingLabel(key: string): string {
  const localized = t(`settings.items.${key}.label`);
  return localized !== `settings.items.${key}.label` ? localized : key;
}

function settingDesc(setting: SystemSetting): string {
  const localized = t(`settings.items.${setting.key}.desc`);
  return localized !== `settings.items.${setting.key}.desc` ? localized : setting.description;
}

// Fallback list: if the backend ever returns settings without a `type` field
// (older versions, ad-hoc inserts) we still want a sensible default.
const FALLBACK_BOOL_KEYS = ['server_dark_mode', 'dns_enable_logging', 'dhcp_enable_failover'];
const FALLBACK_NUMBER_KEYS = ['dns_default_ttl', 'dns_cache_max_entries', 'dhcp_default_lease_time', 'dhcp_max_lease_time'];

function effectiveType(setting: SystemSetting): 'bool' | 'int' | 'string' {
  if (setting.type === 'bool' || setting.type === 'int' || setting.type === 'string') return setting.type;
  if (FALLBACK_BOOL_KEYS.includes(setting.key) || setting.value === 'true' || setting.value === 'false') return 'bool';
  if (FALLBACK_NUMBER_KEYS.includes(setting.key)) return 'int';
  if (typeof setting.value === 'string' && /^\d+$/.test(setting.value)) return 'int';
  return 'string';
}

function isLongValue(value: string): boolean {
  return !!value && value.length > 100;
}

async function loadData() {
  loading.value = true;
  try {
    const result = await listSystemSettings();
    settings.value = result.data;
    for (const s of result.data) {
      if (effectiveType(s) === 'int') numericValues[s.key] = Number(s.value);
    }
  } catch (err) {
    void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
  } finally {
    loading.value = false;
  }
}

async function handleSave(setting: SystemSetting) {
  if (!perm.canWrite('settings')) return;
  savingKeys[setting.key] = true;
  try {
    const valueToSave =
      effectiveType(setting) === 'int' ? String(numericValues[setting.key] ?? setting.value) : setting.value;
    await updateSystemSetting(setting.key, valueToSave);
    void MessagePlugin.success(t('settings.updateSuccess'));
  } catch (err) {
    void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
  } finally {
    savingKeys[setting.key] = false;
  }
}

onMounted(loadData);
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('settings.title') }}</h2>
    </TCard>

    <TLoading :loading="loading" size="small">
      <TCard :bordered="false">
        <TEmpty v-if="!loading && !settings.length" :description="t('common.noData')" />
        <div v-for="setting in settings" :key="setting.key" class="setting-row">
          <div class="setting-meta">
            <div class="setting-label">{{ settingLabel(setting.key) }}</div>
            <div class="setting-desc">{{ settingDesc(setting) }}</div>
          </div>

          <div class="setting-control">
            <TSwitch
              v-if="effectiveType(setting) === 'bool'"
              v-model="setting.value"
              :custom-value="['true', 'false']"
              :disabled="!perm.canWrite('settings')"
            />
            <TInputNumber
              v-else-if="effectiveType(setting) === 'int'"
              v-model="numericValues[setting.key]"
              :min="0"
              :disabled="!perm.canWrite('settings')"
              style="width: 220px"
            />
            <TTextarea
              v-else-if="isLongValue(String(setting.value))"
              v-model="setting.value"
              :disabled="!perm.canWrite('settings')"
              :autosize="{ minRows: 2, maxRows: 6 }"
              style="width: 320px"
            />
            <TInput
              v-else
              v-model="setting.value"
              :disabled="!perm.canWrite('settings')"
              style="width: 320px"
            />
            <TButton
              v-if="perm.canWrite('settings')"
              theme="primary"
              size="small"
              :loading="savingKeys[setting.key]"
              @click="handleSave(setting)"
            >
              {{ t('common.save') }}
            </TButton>
          </div>
        </div>
      </TCard>
    </TLoading>
  </div>
</template>

<style scoped>
.td-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.td-page-header :deep(.t-card__body) {
  padding: 16px 20px;
}
.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  padding: 14px 0;
  border-bottom: 1px solid #f0f0f0;
}
.setting-row:last-child {
  border-bottom: none;
}
.setting-meta {
  min-width: 220px;
}
.setting-label {
  font-size: 14px;
  font-weight: 500;
}
.setting-desc {
  margin-top: 4px;
  font-size: 12px;
  color: #9ca3af;
}
.setting-control {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
</style>