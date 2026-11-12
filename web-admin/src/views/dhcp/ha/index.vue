<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import { getHAStatus, type DHCPHAStatus } from '@/service/api/goddi/ha';

defineOptions({ name: 'DHCPHAPage' });

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
    { label: t('haStatus.configuredRole'), value: s.configured_role ?? '-' },
    { label: t('haStatus.roleInForce'), value: s.role_in_force ?? s.role ?? '-' },
    { label: t('haStatus.sequence'), value: s.sequence ?? '-' },
    { label: t('haStatus.acknowledged'), value: s.acknowledged_sequence ?? '-' },
    { label: t('haStatus.peerApplied'), value: s.peer_applied_sequence ?? s.applied_sequence ?? '-' },
    { label: t('haStatus.peerFacts'), value: s.peer_applied_facts_sequence ?? '-' },
    { label: t('haStatus.peerLastSeen'), value: s.peer_last_seen ?? '-' }
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
      <div class="ha-header">
        <div>
          <h2>{{ t('route.dhcp_ha') }}</h2>
          <p class="ha-subtitle">{{ t('haStatus.subtitle') }}</p>
        </div>
        <div class="ha-actions">
          <TSwitch v-model="autoRefresh" :label="[t('haStatus.autoOn'), t('haStatus.autoOff')]" />
          <TButton theme="primary" variant="outline" :loading="loading" @click="refresh">
            {{ t('common.refresh') }}
          </TButton>
        </div>
      </div>
    </TCard>

    <TAlert v-if="error" theme="error" :message="error" class="ha-alert" />

    <TCard v-if="status && !status.ha_enabled" :bordered="false">
      <TEmpty :description="t('haStatus.standalone')">
        <template #image><TIcon name="server" size="48px" /></template>
        <template #action>
          <TTag theme="default" variant="light-outline">standalone</TTag>
        </template>
      </TEmpty>
    </TCard>

    <TCard v-else-if="status" :bordered="false">
      <div class="ha-state-row">
        <div class="ha-state-item">
          <span class="ha-label">{{ t('haStatus.state') }}</span>
          <TTag :theme="stateTheme" variant="light" size="large">{{ status.state ?? '-' }}</TTag>
        </div>
        <div class="ha-state-item">
          <span class="ha-label">{{ t('haStatus.redundant') }}</span>
          <TTag :theme="boolTheme(status.redundant)" variant="light" size="large">
            {{ status.redundant === true ? 'true' : status.redundant === false ? 'false' : '-' }}
          </TTag>
        </div>
        <div class="ha-state-item">
          <span class="ha-label">{{ t('haStatus.promising') }}</span>
          <TTag :theme="boolTheme(status.promising)" variant="light" size="large">
            {{ status.promising === true ? 'true' : status.promising === false ? 'false' : '-' }}
          </TTag>
        </div>
      </div>

      <div class="ha-grid">
        <div v-for="row in rows" :key="row.label" class="ha-cell">
          <span class="ha-label">{{ row.label }}</span>
          <span class="ha-value">{{ row.value }}</span>
        </div>
      </div>
    </TCard>

    <TCard v-else :bordered="false" :loading="loading">
      <TEmpty :description="t('common.loading')" />
    </TCard>

    <TCard :bordered="false" class="ha-source">
      <span>{{ t('haStatus.source') }}</span>
      <code>GET /api/v1/dhcp/ha</code>
      <span class="ha-source-sep">·</span>
      <span>{{ t('haStatus.readOnly') }}</span>
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
  padding: 16px 20px;
}
.ha-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}
.ha-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
.ha-subtitle {
  margin: 6px 0 0;
  color: var(--td-text-color-secondary);
  font-size: 13px;
}
.ha-actions {
  display: flex;
  align-items: center;
  gap: 16px;
}
.ha-alert {
  margin-bottom: 0;
}
.ha-state-row {
  display: flex;
  gap: 32px;
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
.ha-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 12px 24px;
}
.ha-cell {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px dashed var(--td-component-stroke);
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
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  color: var(--td-text-color-secondary);
  font-size: 12px;
}
.ha-source code {
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--td-bg-color-component);
}
.ha-source-sep {
  opacity: 0.6;
}
</style>
