<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import { getHAStatus, type DHCPHAStatus } from '@/service/api/goddi/ha';
import ConfigReplicationAlert from '@/components/ConfigReplicationAlert.vue';
import { formatAPITime } from '@/utils/datetime';
import { haRoleLabel, haStateLabel } from '@/composables/useDhcpStatusLabels';

defineOptions({ name: 'DHCPHAPage' });

/**
 * Starter-style detail page: TCard page header + TAlert banners +
 * TDescriptions detail grid + TEmpty states. The read-only HA status
 * contract, auto-refresh timer and ConfigReplicationAlert are unchanged.
 */

const { t } = useI18n();

const status = ref<DHCPHAStatus | null>(null);
const loading = ref(false);
const error = ref('');
const autoRefresh = ref(true);
let timer: number | undefined;

async function load(silent = false) {
  if (!silent) loading.value = true;
  try {
    status.value = await getHAStatus();
    error.value = '';
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
    if (!silent) void MessagePlugin.error(error.value);
  } finally {
    loading.value = false;
  }
}

function refresh() {
  void load();
}

const stateTheme = computed(() => {
  const s = status.value?.state ?? '';
  if (s === 'primary') return 'success';
  if (s === 'primary-degraded' || s === 'paused') return 'warning';
  if (s === 'fenced') return 'danger';
  return 'default';
});

const boolTheme = (v: boolean | undefined) => (v ? 'success' : v === false ? 'warning' : 'default');

const rows = computed(() => {
  const s = status.value;
  if (!s || !s.ha_enabled) return [];
  return [
    { label: t('haStatus.nodeId'), value: s.node_id ?? '-' },
    { label: t('haStatus.configuredRole'), value: s.configured_role ? haRoleLabel(t, s.configured_role) : '-' },
    { label: t('haStatus.roleInForce'), value: s.role_in_force || s.role ? haRoleLabel(t, s.role_in_force || s.role || '') : '-' },
    { label: t('haStatus.sequence'), value: s.sequence ?? '-' },
    { label: t('haStatus.acknowledged'), value: s.acknowledged_sequence ?? '-' },
    { label: t('haStatus.peerApplied'), value: s.peer_applied_sequence ?? s.applied_sequence ?? '-' },
    { label: t('haStatus.peerFacts'), value: s.peer_applied_facts_sequence ?? '-' },
    { label: t('haStatus.peerLastSeen'), value: formatAPITime(s.peer_last_seen) }
  ];
});

onMounted(() => {
  void load();
  timer = window.setInterval(() => {
    if (autoRefresh.value) void load(true);
  }, 5000);
});

onUnmounted(() => {
  if (timer !== undefined) window.clearInterval(timer);
});
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <div class="td-page-title">
        <div>
          <h2>{{ t('route.dhcp_ha') }}</h2>
          <p class="ha-subtitle">{{ t('haStatus.subtitle') }}</p>
        </div>
      </div>
      <TSpace>
        <TSwitch v-model="autoRefresh" :label="[t('haStatus.autoOn'), t('haStatus.autoOff')]" />
        <TButton theme="primary" variant="outline" :loading="loading" @click="refresh">
          {{ t('common.refresh') }}
        </TButton>
      </TSpace>
    </TCard>

    <ConfigReplicationAlert />

    <TAlert v-if="error && status" theme="error" role="alert" :message="error" close class="ha-alert" @close="error = ''">
      <template #operation>
        <TButton size="small" theme="primary" variant="outline" @click="refresh">{{ t('common.retry') }}</TButton>
      </template>
    </TAlert>

    <TCard v-if="status && !status.ha_enabled" :bordered="false">
      <TEmpty :description="t('haStatus.standalone')">
        <template #image><TIcon name="server" size="48px" /></template>
        <template #action>
          <TTag theme="default" variant="light-outline">{{ t('common.standalone') }}</TTag>
        </template>
      </TEmpty>
    </TCard>

    <TCard v-else-if="status" :bordered="false">
      <TSpace class="ha-state-row" size="large">
        <div class="ha-state-item">
          <span class="ha-label">{{ t('haStatus.state') }}</span>
          <TTag :theme="stateTheme" variant="light" size="large">{{ status.state ? haStateLabel(t, status.state) : '-' }}</TTag>
        </div>
        <div class="ha-state-item">
          <span class="ha-label">{{ t('haStatus.redundant') }}</span>
          <TTag :theme="boolTheme(status.redundant)" variant="light" size="large">
            {{ status.redundant === true ? t('common.yes') : status.redundant === false ? t('common.no') : '-' }}
          </TTag>
        </div>
        <div class="ha-state-item">
          <span class="ha-label">{{ t('haStatus.promising') }}</span>
          <TTag :theme="boolTheme(status.promising)" variant="light" size="large">
            {{ status.promising === true ? t('common.yes') : status.promising === false ? t('common.no') : '-' }}
          </TTag>
        </div>
      </TSpace>

      <TDescriptions :column="2" bordered size="small" class="ha-descriptions">
        <TDescriptionsItem v-for="row in rows" :key="row.label" :label="row.label">
          <span class="ha-value">{{ row.value }}</span>
        </TDescriptionsItem>
      </TDescriptions>
    </TCard>

    <TCard v-else :bordered="false" :loading="loading">
      <TAlert v-if="error" theme="error" role="alert" :message="error">
        <template #operation>
          <TButton size="small" theme="primary" variant="outline" @click="refresh">{{ t('common.retry') }}</TButton>
        </template>
      </TAlert>
    </TCard>

    <TCard :bordered="false" class="ha-source">
      <TSpace size="small">
        <span>{{ t('haStatus.source') }}</span>
        <TTag theme="default" variant="light-outline">GET /api/v1/dhcp/ha</TTag>
        <span class="ha-source-sep">·</span>
        <span>{{ t('haStatus.readOnly') }}</span>
      </TSpace>
    </TCard>
  </div>
</template>

<style scoped>
.td-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.td-page-header :deep(.t-card__body) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 20px;
}
.ha-subtitle {
  margin: 6px 0 0;
  color: var(--td-text-color-secondary);
  font-size: 13px;
}
.ha-alert {
  margin-bottom: 0;
}
.ha-state-row {
  flex-wrap: wrap;
  flex-wrap: wrap;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--td-component-stroke);
  margin-bottom: 16px;
}
.ha-state-item {
  display: flex;
  align-items: center;
  gap: 10px;
}
.ha-label {
  color: var(--td-text-color-secondary);
  font-size: 13px;
}
.ha-value {
  font-family: var(--td-font-family-mono, monospace);
  font-size: 13px;
  word-break: break-all;
}
.ha-source {
  color: var(--td-text-color-secondary);
  font-size: 12px;
}
.ha-source-sep {
  opacity: 0.6;
}
</style>