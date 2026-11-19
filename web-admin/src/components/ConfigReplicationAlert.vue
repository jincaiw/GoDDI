<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { getConfigReplicationStatus, type ConfigReplicationStatus } from '@/service/api/goddi/ha';

/**
 * Configuration replication health banner. Rendered on the dashboard and the
 * HA page so a failed/stale follower cannot look healthy. Only the follower
 * role has a replication watermark; the author and standalone nodes render
 * nothing instead of a misleading green success state.
 */
defineOptions({ name: 'ConfigReplicationAlert' });

const { t } = useI18n();
const status = ref<ConfigReplicationStatus | null>(null);
const loadError = ref('');
let timer: number | undefined;

async function refresh() {
  try {
    status.value = await getConfigReplicationStatus();
    loadError.value = '';
  } catch (e) {
    loadError.value = e instanceof Error ? e.message : String(e);
  }
}

const alertMessage = computed(() => {
  if (loadError.value) {
    return t('haStatus.replLoadError') + '：' + loadError.value;
  }
  const s = status.value;
  if (!s || s.healthy) return '';
  const reason = s.last_error || t('haStatus.replUnknown');
  if (s.role === 'author') {
    return t('haStatus.replAlertTitle') + '：' + reason;
  }
  if (s.role !== 'follower') return '';
  return (
    t('haStatus.replAlertTitle') + '：' + reason +
    '（' + t('haStatus.replLastSync') + ' ' + (s.synced_at || '—') +
    '，' + t('haStatus.replSource') + ' ' + (s.source || '—') + '）。' +
    t('haStatus.replAlertHint')
  );
});

// A healthy watermark can still be behind the author's current version.
// Show that difference explicitly instead of reporting an unqualified green.
const behindMessage = computed(() => {
  const s = status.value;
  if (!s || !s.healthy || s.role !== 'follower') return '';
  if (!s.expected_version || s.expected_version === s.applied_version) return '';
  return (
    t('haStatus.replBehindTitle') + '：' +
    t('haStatus.replApplied') + ' ' + s.applied_version + ' / ' +
    t('haStatus.replExpected') + ' ' + s.expected_version + '。' +
    t('haStatus.replBehindHint')
  );
});

const healthyLine = computed(() => {
  const s = status.value;
  if (!s || !s.healthy) return '';
  if (s.role === 'author') {
    return s.expected_version
      ? t('haStatus.replExpected') + ' ' + s.expected_version
      : '';
  }
  if (s.role !== 'follower') return '';
  const versions = s.expected_version
    ? ' · ' + t('haStatus.replApplied') + ' ' + s.applied_version +
      ' / ' + t('haStatus.replExpected') + ' ' + s.expected_version
    : '';
  return t('haStatus.replHealthy') + versions + ' · ' + t('haStatus.replLastSync') + ' ' + (s.synced_at || '—');
});
onMounted(() => {
  void refresh();
  timer = window.setInterval(() => void refresh(), 10000);
});
onUnmounted(() => {
  if (timer !== undefined) window.clearInterval(timer);
});
</script>

<template>
  <!--
    Decision (task 3): a healthy replication watermark is not rendered as a
    full-width banner because it breaks the dashboard/base visual rhythm and
    duplicates information already on the HA page. Degraded, behind and error
    states still render a prominent alert, so fail-closed visibility is kept.
  -->
  <TAlert v-if="alertMessage" theme="warning" :message="alertMessage" class="repl-alert" />
  <TAlert v-else-if="behindMessage" theme="warning" :message="behindMessage" class="repl-alert" />
  <span v-else-if="healthyLine" class="repl-ok" role="status">{{ healthyLine }}</span>
</template>

<style scoped>
.repl-alert {
  margin-bottom: 16px;
}
.repl-ok {
  display: block;
  margin-bottom: 4px;
  color: var(--td-text-color-placeholder);
  font-size: 12px;
}
</style>