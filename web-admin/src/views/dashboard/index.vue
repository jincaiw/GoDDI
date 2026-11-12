<script setup lang="ts">
import { defineAsyncComponent, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import { GlobeOutline, ServerOutline, DesktopOutline, GridOutline } from '@vicons/ionicons5';
import {
  getDNSStats,
  getDashboardStats,
  getDashboardTop,
  type HourlyDNSStats,
  type StatsResponse,
  type TopStats
} from '@/service/api/goddi/dns';
import { listAuditLogs } from '@/service/api/goddi/logs';

defineOptions({ name: 'DashboardPage' });

const DashboardChart = defineAsyncComponent(() => import('@/components/DashboardChart.vue'));
const StatsTrendChart = defineAsyncComponent(() => import('@/components/StatsTrendChart.vue'));

const { t, locale } = useI18n();

const stats = ref({ dnsQueries: 0, cacheHitRate: 0, activeLeases: 0, ipamUsage: 0 });
const chartReady = ref(false);
const chartData = ref<{ hours: string[]; queries: number[] }>({ hours: [], queries: [] });

const topRange = ref<'hour' | 'day' | 'week'>('hour');
const topRangeOptions = [
  { label: t('dashboard.hour'), value: 'hour' },
  { label: t('dashboard.day'), value: 'day' },
  { label: t('dashboard.week'), value: 'week' }
];
const topStats = ref<TopStats>({
  range: 'hour',
  top_clients: [],
  top_domains: [],
  top_blocked: [],
  rcodes: { total: 0, noerror: 0, nxdomain: 0, servfail: 0, refused: 0, other: 0 }
});

async function loadTop() {
  try { topStats.value = await getDashboardTop(topRange.value, 10); } catch { /* ignore */ }
}

const statsRange = ref<'hour' | 'day' | 'week' | 'month' | 'year'>('day');
const statsRangeOptions = [
  { label: t('dashboard.hour'), value: 'hour' },
  { label: t('dashboard.day'), value: 'day' },
  { label: t('dashboard.week'), value: 'week' },
  { label: t('dashboard.month'), value: 'month' },
  { label: t('dashboard.year'), value: 'year' }
];
const ltStats = ref<StatsResponse | null>(null);

async function loadLongTermStats() {
  try { ltStats.value = await getDNSStats({ range: statsRange.value }); } catch { /* ignore */ }
}

const eventColumns: PrimaryTableCol[] = [
  { colKey: 'username', title: t('logs.audit.user'), width: 110 },
  { colKey: 'action', title: t('logs.audit.action'), width: 110 },
  { colKey: 'resource_type', title: t('logs.audit.resource'), width: 130 },
  {
    colKey: 'created_at',
    title: t('common.createdAt'),
    width: 170,
    cell: (_h, { row }) => {
      const value = String((row as { created_at?: string }).created_at ?? '');
      const date = new Date(value);
      return Number.isNaN(date.getTime())
        ? value
        : date.toLocaleString(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false });
    }
  }
];

const recentEvents = ref<unknown[]>([]);

onMounted(async () => {
  void loadTop();
  void loadLongTermStats();
  try {
    const [dashboardResult, auditResult] = await Promise.allSettled([
      getDashboardStats(),
      listAuditLogs({ page: 1, page_size: 10 })
    ]);

    if (dashboardResult.status === 'fulfilled') {
      const d = dashboardResult.value;
      stats.value.dnsQueries = d.dns_queries_today;
      stats.value.cacheHitRate = Math.round(d.cache_hit_rate);
      stats.value.activeLeases = d.active_leases;
      stats.value.ipamUsage = Math.round(d.ipam_usage);

      const hourlyData = d.recent_dns_stats as HourlyDNSStats[];
      if (hourlyData && hourlyData.length > 0) {
        const hours = hourlyData.map(item => {
          const parts = item.hour.split(' ');
          const timePart = parts[1] ?? item.hour;
          return timePart.substring(0, 5);
        });
        chartData.value = { hours, queries: hourlyData.map(item => item.queries) };
      }
    }

    if (auditResult.status === 'fulfilled') {
      recentEvents.value = auditResult.value.data;
    }
  } catch {
    // Silently handle dashboard errors
  } finally {
    chartReady.value = true;
  }
});
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('dashboard.title') }}</h2>
      <TTag theme="primary" variant="light">Single-Node</TTag>
    </TCard>

    <div class="metric-grid">
      <TCard :bordered="false" class="metric-card">
        <div class="metric-inner">
          <span class="metric-icon" style="color: #0a84ff"><GlobeOutline /></span>
          <div class="metric-text">
            <span class="metric-label">{{ t('dashboard.dnsQueriesToday') }}</span>
            <b class="metric-value">{{ stats.dnsQueries.toLocaleString('en-US') }}</b>
          </div>
        </div>
      </TCard>
      <TCard :bordered="false" class="metric-card">
        <div class="metric-inner">
          <span class="metric-icon" style="color: #af52de"><ServerOutline /></span>
          <div class="metric-text">
            <span class="metric-label">{{ t('dashboard.cacheHitRate') }}</span>
            <b class="metric-value">{{ stats.cacheHitRate }}%</b>
          </div>
        </div>
      </TCard>
      <TCard :bordered="false" class="metric-card">
        <div class="metric-inner">
          <span class="metric-icon" style="color: #ff9500"><DesktopOutline /></span>
          <div class="metric-text">
            <span class="metric-label">{{ t('dashboard.activeLeases') }}</span>
            <b class="metric-value">{{ stats.activeLeases.toLocaleString('en-US') }}</b>
          </div>
        </div>
      </TCard>
      <TCard :bordered="false" class="metric-card">
        <div class="metric-inner">
          <span class="metric-icon" style="color: #34c759"><GridOutline /></span>
          <div class="metric-text">
            <span class="metric-label">{{ t('dashboard.ipamUsage') }}</span>
            <b class="metric-value">{{ stats.ipamUsage }}%</b>
          </div>
        </div>
      </TCard>
    </div>

    <div class="chart-grid">
      <TCard :bordered="false" :title="t('dashboard.dnsQueryTrend')">
        <TLoading :loading="!chartReady" size="small">
          <DashboardChart v-if="chartReady" :hours="chartData.hours" :queries="chartData.queries" />
        </TLoading>
      </TCard>
      <TCard :bordered="false">
        <template #title>
          <div class="card-title-row">
            <span>{{ t('dashboard.dnsStats') }}</span>
            <TSelect v-model="statsRange" :options="statsRangeOptions" size="small" style="width: 130px" @change="loadLongTermStats" />
          </div>
        </template>
        <StatsTrendChart :stats="ltStats" />
      </TCard>
    </div>

    <div class="top-grid">
      <TCard :bordered="false">
        <template #title>
          <div class="card-title-row">
            <span>{{ t('dashboard.topStats') }}</span>
            <TRadioGroup v-model="topRange" :options="topRangeOptions" size="small" variant="default-filled" @change="loadTop" />
          </div>
        </template>
        <div class="top-columns">
          <div>
            <p class="top-title">{{ t('dashboard.topClients') }}</p>
            <ul class="top-list">
              <li v-for="(item, index) in topStats.top_clients" :key="index">
                <span class="top-name">{{ item.name }}</span><span class="top-count">{{ item.count }}</span>
              </li>
              <li v-if="!topStats.top_clients.length" class="top-empty">—</li>
            </ul>
          </div>
          <div>
            <p class="top-title">{{ t('dashboard.topDomains') }}</p>
            <ul class="top-list">
              <li v-for="(item, index) in topStats.top_domains" :key="index">
                <span class="top-name">{{ item.name }}</span><span class="top-count">{{ item.count }}</span>
              </li>
              <li v-if="!topStats.top_domains.length" class="top-empty">—</li>
            </ul>
          </div>
          <div>
            <p class="top-title">{{ t('dashboard.topBlocked') }}</p>
            <ul class="top-list">
              <li v-for="(item, index) in topStats.top_blocked" :key="index">
                <span class="top-name">{{ item.name }}</span><span class="top-count">{{ item.count }}</span>
              </li>
              <li v-if="!topStats.top_blocked.length" class="top-empty">—</li>
            </ul>
          </div>
        </div>
      </TCard>

      <TCard :bordered="false" :title="t('dashboard.rcodeDistribution')">
        <div class="rcode-grid">
          <div v-for="(value, key) in topStats.rcodes" :key="key" class="rcode-item">
            <span class="rcode-label">{{ key }}</span>
            <b class="rcode-value">{{ value }}</b>
          </div>
        </div>
      </TCard>
    </div>

    <TCard :bordered="false" :title="t('dashboard.recentEvents')">
      <TTable :columns="eventColumns" :data="recentEvents" row-key="created_at" size="small" stripe hover />
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
  padding: 16px 20px;
}
.td-page-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
.metric-grid,
.chart-grid,
.top-grid {
  display: grid;
  gap: 16px;
}
.metric-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}
.chart-grid,
.top-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
@media (max-width: 1100px) {
  .metric-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .chart-grid, .top-grid { grid-template-columns: 1fr; }
}
.metric-inner {
  display: flex;
  align-items: center;
  gap: 16px;
}
.metric-icon {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border-radius: 10px;
  background: currentColor;
  flex: 0 0 auto;
}
.metric-icon :deep(svg) {
  width: 24px;
  height: 24px;
  color: #fff;
}
.metric-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.metric-label {
  font-size: 13px;
  color: #6b7280;
}
.metric-value {
  font-size: 26px;
  font-weight: 600;
  letter-spacing: -0.5px;
}
.card-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}
.top-columns {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}
.top-title {
  margin: 0 0 8px;
  font-size: 13px;
  color: #888;
}
.top-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.top-list li {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  padding: 4px 0;
  font-size: 13px;
  border-bottom: 1px dashed rgba(128, 128, 128, 0.15);
}
.top-list li:last-child { border-bottom: none; }
.top-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 75%;
}
.top-count { font-weight: 600; }
.top-empty { color: #9ca3af; }
.rcode-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}
.rcode-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px;
  border-radius: 8px;
  background: rgba(128, 128, 128, 0.08);
}
.rcode-label {
  font-size: 12px;
  color: #6b7280;
}
.rcode-value {
  font-size: 20px;
}
</style>
