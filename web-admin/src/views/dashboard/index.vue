<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Link as TLink } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import { DesktopOutline, GridOutline } from '@vicons/ionicons5';
import {
  getDNSStats,
  getDashboardStats,
  getDashboardTop,
  type HourlyDNSStats,
  type StatsResponse,
  type TopStats
} from '@/service/api/goddi/dns';
import { listAuditLogs } from '@/service/api/goddi/logs';
import ConfigReplicationAlert from '@/components/ConfigReplicationAlert.vue';
import DashboardSparkline from '@/components/DashboardSparkline.vue';
import DashboardTrend from '@/components/DashboardTrend.vue';
import StatsTrendChart from '@/components/StatsTrendChart.vue';
import DashboardChart from '@/components/DashboardChart.vue';

defineOptions({ name: 'DashboardPage' });

const { t, locale } = useI18n();

const stats = ref({ dnsQueries: 0, cacheHitRate: 0, activeLeases: 0, ipamUsage: 0 });
const chartReady = ref(false);
const chartData = ref<{ hours: string[]; queries: number[] }>({ hours: [], queries: [] });
const dashboardError = ref('');
const topError = ref('');
const statsError = ref('');
const auditError = ref('');

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : t('common.failed');
}

function rcodeLabel(key: string) {
  const keys: Record<string, string> = {
    total: 'dashboard.statsTotal',
    noerror: 'dashboard.statsNoError',
    nxdomain: 'dashboard.statsNxDomain',
    servfail: 'dashboard.statsServFail',
    refused: 'dashboard.statsRefused'
  };
  return t((keys[key] || 'dashboard.rcodesOther') as any);
}

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
  topError.value = '';
  try {
    topStats.value = await getDashboardTop(topRange.value, 10);
  } catch (err) {
    topError.value = errorText(err);
  }
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
  statsError.value = '';
  try {
    ltStats.value = await getDNSStats({ range: statsRange.value });
  } catch (err) {
    statsError.value = errorText(err);
  }
}

const cacheSeries = computed(() =>
  (ltStats.value?.series ?? []).map(point => (point.total > 0 ? Math.round((point.cached / point.total) * 100) : 0))
);

function trendOf(values: number[]): string {
  const positive = values.filter(value => Number(value) > 0);
  if (positive.length < 2) return '0%';
  const first = positive[0];
  const last = positive[positive.length - 1];
  const rate = ((last - first) / first) * 100;
  return `${rate >= 0 ? '+' : ''}${rate.toFixed(1)}%`;
}

const dnsTrend = computed(() => trendOf(chartData.value.queries));
const cacheTrend = computed(() => trendOf(cacheSeries.value));

function rankCell(h2: typeof h, row: Record<string, unknown>) {
  const index = Number(row.rankIndex ?? 1);
  return h2('span', { class: ['dashboard-rank__cell', index <= 3 ? 'dashboard-rank__cell--top' : ''] }, String(index));
}

const rankColumns: PrimaryTableCol[] = [
  { align: 'center', colKey: 'rankIndex', title: t('dashboard.rank'), width: 70, cell: (_h, { row }) => rankCell(h, row as Record<string, unknown>) },
  { align: 'left', ellipsis: true, colKey: 'name', title: t('dashboard.topClients'), minWidth: 160 },
  { align: 'center', colKey: 'count', title: t('dashboard.count'), width: 90 },
  { align: 'center', colKey: 'operation', title: t('dashboard.trend'), width: 90, cell: () => h(TLink, { theme: 'primary' }, () => t('dashboard.rankInfo')) }
];

const rankData = computed(() =>
  topStats.value.top_clients.map((item, index) => ({ rankIndex: index + 1, name: item.name, count: item.count }))
);

const domainColumns: PrimaryTableCol[] = [
  { align: 'center', colKey: 'rankIndex', title: t('dashboard.rank'), width: 70, cell: (_h, { row }) => rankCell(h, row as Record<string, unknown>) },
  { align: 'left', ellipsis: true, colKey: 'name', title: t('dashboard.topDomains'), minWidth: 160 },
  { align: 'center', colKey: 'count', title: t('dashboard.count'), width: 90 },
  { align: 'center', colKey: 'operation', title: t('dashboard.trend'), width: 90, cell: () => h(TLink, { theme: 'primary' }, () => t('dashboard.rankInfo')) }
];

const domainData = computed(() =>
  topStats.value.top_domains.map((item, index) => ({ rankIndex: index + 1, name: item.name, count: item.count }))
);

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

async function loadAll() {
  dashboardError.value = '';
  auditError.value = '';
  chartReady.value = false;
  await Promise.all([loadTop(), loadLongTermStats()]);

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
  } else {
    dashboardError.value = errorText(dashboardResult.reason);
  }

  if (auditResult.status === 'fulfilled') {
    recentEvents.value = auditResult.value.data;
  } else {
    auditError.value = errorText(auditResult.reason);
  }
  chartReady.value = true;
}

onMounted(loadAll);
</script>

<template>
  <div class="td-page dashboard-base">
    <!--
 dashboard/base starts directly with TopPanel; the shell already
         carries the page title and HA badge in the header.
-->
    <ConfigReplicationAlert />
    <TAlert
      v-if="dashboardError || topError || statsError || auditError"
      theme="error"
      role="alert"
      class="dashboard-error"
      :message="dashboardError || topError || statsError || auditError"
    >
      <template #operation>
        <TButton size="small" theme="primary" variant="outline" @click="loadAll">{{ t('common.retry') }}</TButton>
      </template>
    </TAlert>

    <!-- TopPanel: four KPI cards, first one on the brand background. -->
    <TRow :gutter="[16, 16]" class="row-container">
      <TCol :xs="6" :xl="3">
        <TCard :title="t('dashboard.dnsQueriesToday')" :bordered="false" class="dashboard-item dashboard-item--main-color">
          <div class="dashboard-item-top"><span>{{ stats.dnsQueries.toLocaleString('en-US') }}</span></div>
          <div class="dashboard-item-left"><DashboardSparkline :values="chartData.queries" type="line" inverse /></div>
          <template #footer>
            <div class="dashboard-item-bottom">
              <div class="dashboard-item-block">
                <span>{{ t('dashboard.sinceLastWeek') }}</span>
                <DashboardTrend :value="dnsTrend" inverse />
              </div>
              <TIcon name="chevron-right" />
            </div>
          </template>
        </TCard>
      </TCol>
      <TCol :xs="6" :xl="3">
        <TCard :title="t('dashboard.cacheHitRate')" :bordered="false" class="dashboard-item">
          <div class="dashboard-item-top"><span>{{ stats.cacheHitRate }}%</span></div>
          <div class="dashboard-item-left"><DashboardSparkline :values="cacheSeries" type="bar" /></div>
          <template #footer>
            <div class="dashboard-item-bottom">
              <div class="dashboard-item-block">
                <span>{{ t('dashboard.sinceLastWeek') }}</span>
                <DashboardTrend :value="cacheTrend" />
              </div>
              <TIcon name="chevron-right" />
            </div>
          </template>
        </TCard>
      </TCol>
      <TCol :xs="6" :xl="3">
        <TCard :title="t('dashboard.activeLeases')" :bordered="false" class="dashboard-item">
          <div class="dashboard-item-top"><span>{{ stats.activeLeases.toLocaleString('en-US') }}</span></div>
          <div class="dashboard-item-left"><span class="dashboard-item-icon"><DesktopOutline /></span></div>
          <template #footer>
            <div class="dashboard-item-bottom">
              <div class="dashboard-item-block"><span>{{ t('dashboard.sinceLastWeek') }}</span><DashboardTrend value="0%" /></div>
              <TIcon name="chevron-right" />
            </div>
          </template>
        </TCard>
      </TCol>
      <TCol :xs="6" :xl="3">
        <TCard :title="t('dashboard.ipamUsage')" :bordered="false" class="dashboard-item">
          <div class="dashboard-item-top"><span>{{ stats.ipamUsage }}%</span></div>
          <div class="dashboard-item-left"><span class="dashboard-item-icon"><GridOutline /></span></div>
          <template #footer>
            <div class="dashboard-item-bottom">
              <div class="dashboard-item-block"><span>{{ t('dashboard.sinceLastWeek') }}</span><DashboardTrend value="0%" /></div>
              <TIcon name="chevron-right" />
            </div>
          </template>
        </TCard>
      </TCol>
    </TRow>

    <!-- MiddleChart: full-width DNS analytics card. -->
    <TCard :bordered="false" class="dashboard-chart-card row-container">
      <template #title>
        <div class="card-title-row">
          <span>{{ t('dashboard.dnsStats') }}</span>
          <TSelect v-model="statsRange" :options="statsRangeOptions" size="small" style="width: 130px" @change="loadLongTermStats" />
        </div>
      </template>
      <TLoading :loading="!chartReady" size="small">
        <StatsTrendChart v-if="chartReady" :stats="ltStats" />
      </TLoading>
    </TCard>

    <!-- RankList: two side-by-side ranking tables. -->
    <TRow :gutter="[16, 16]" class="row-container">
      <TCol :xs="12" :xl="6">
        <TCard :title="t('dashboard.topClients')" :bordered="false" class="dashboard-rank-card">
          <template #actions>
            <TRadioGroup v-model="topRange" :options="topRangeOptions" size="small" variant="default-filled" @change="loadTop" />
          </template>
          <TTable :columns="rankColumns" :data="rankData" row-key="name" size="small" />
        </TCard>
      </TCol>
      <TCol :xs="12" :xl="6">
        <TCard :title="t('dashboard.topDomains')" :bordered="false" class="dashboard-rank-card">
          <template #actions>
            <TRadioGroup v-model="topRange" :options="topRangeOptions" size="small" variant="default-filled" @change="loadTop" />
          </template>
          <TTable :columns="domainColumns" :data="domainData" row-key="name" size="small" />
        </TCard>
      </TCol>
    </TRow>

    <!-- OutputOverview: wide DNS trend + response-code / blocked summary panel. -->
    <TRow :gutter="[16, 16]" class="row-container">
      <TCol :xs="12" :xl="9">
        <TCard :bordered="false" :title="t('dashboard.dnsQueryTrend')" class="dashboard-overview-card overview-panel">
          <TLoading :loading="!chartReady" size="small">
            <DashboardChart v-if="chartReady" :hours="chartData.hours" :queries="chartData.queries" />
          </TLoading>
        </TCard>
      </TCol>
      <TCol :xs="12" :xl="3">
        <TCard :bordered="false" class="dashboard-overview-card export-panel">
          <template #actions>
            <TButton size="small" @click="loadTop">{{ t('common.refresh') }}</TButton>
          </template>
          <TRow :gutter="[8, 8]">
            <TCol :xs="6" :xl="12">
              <TCard :bordered="false" :subtitle="t('dashboard.rcodeNoError')" class="inner-card">
                <div class="inner-card__content">
                  <div class="inner-card__content-title">{{ topStats.rcodes.noerror }}</div>
                  <div class="inner-card__content-footer">
                    <span>{{ t('dashboard.statsTotal') }}</span>
                    <span class="trend-tag">{{ topStats.rcodes.total }}</span>
                  </div>
                </div>
              </TCard>
            </TCol>
            <TCol :xs="6" :xl="12">
              <TCard :bordered="false" :subtitle="t('dashboard.topBlocked')" class="inner-card">
                <div class="inner-card__content">
                  <div class="inner-card__content-title">{{ topStats.top_blocked.length }}</div>
                  <div class="inner-card__content-footer">
                    <span>{{ t('dashboard.statsBlocked') }}</span>
                    <span class="trend-tag">{{ topStats.rcodes.nxdomain }}</span>
                  </div>
                </div>
              </TCard>
            </TCol>
          </TRow>
          <div class="rcode-grid">
            <div v-for="(value, key) in topStats.rcodes" :key="key" class="rcode-item">
              <span class="rcode-label">{{ rcodeLabel(String(key)) }}</span>
              <b class="rcode-value">{{ value }}</b>
            </div>
          </div>
        </TCard>
      </TCol>
    </TRow>

    <TCard :bordered="false" class="dashboard-rank-card" :title="t('dashboard.recentEvents')">
      <TTable :columns="eventColumns" :data="recentEvents" row-key="created_at" size="small" stripe hover />
    </TCard>
  </div>
</template>

<style scoped>
.dashboard-base { display: flex; flex-direction: column; gap: 16px; }
.dashboard-error { margin-bottom: 8px; }
.row-container { width: 100%; }

.dashboard-item { padding: var(--td-comp-paddingTB-l) var(--td-comp-paddingLR-l); height: 100%; }
.dashboard-item :deep(.t-card__header), .dashboard-item :deep(.t-card__footer) { padding: 0; }
.dashboard-item :deep(.t-card__title) { font: var(--td-font-body-medium); color: var(--td-text-color-secondary); }
.dashboard-item :deep(.t-card__body) {
  display: flex; flex-direction: column; justify-content: space-between; flex: 1; position: relative;
  padding: 0; margin-top: var(--td-comp-margin-s); margin-bottom: var(--td-comp-margin-l);
}
.dashboard-item-top { display: flex; align-items: flex-start; }
.dashboard-item-top > span { font: var(--td-font-headline-medium); color: var(--td-text-color-primary); }
.dashboard-item-bottom { display: flex; align-items: center; justify-content: space-between; }
.dashboard-item-bottom > .t-icon { font-size: var(--td-comp-size-xxxs); cursor: pointer; }
.dashboard-item-block { display: flex; align-items: center; gap: var(--td-comp-margin-s); color: var(--td-text-color-placeholder); }
.dashboard-item-left { position: absolute; top: 0; right: 0; }
.dashboard-item-icon {
  display: inline-flex; align-items: center; justify-content: center;
  width: var(--td-comp-size-xxxl); height: var(--td-comp-size-xxxl);
  border-radius: 50%; background: var(--td-brand-color-light); color: var(--td-brand-color); font-size: 24px;
}
.dashboard-item--main-color { background: var(--td-brand-color); color: var(--td-text-color-anti); }
.dashboard-item--main-color :deep(.t-card__title),
.dashboard-item--main-color .dashboard-item-top > span,
.dashboard-item--main-color .dashboard-item-bottom,
.dashboard-item--main-color .dashboard-item-block { color: var(--td-text-color-anti); }
.dashboard-item--main-color .dashboard-item-block { opacity: .7; }

.dashboard-chart-card, .dashboard-rank-card, .dashboard-overview-card {
  padding: var(--td-comp-paddingTB-l) var(--td-comp-paddingLR-l);
}
.dashboard-chart-card :deep(.t-card__header), .dashboard-rank-card :deep(.t-card__header), .dashboard-overview-card :deep(.t-card__header) { padding: 0; }
.dashboard-chart-card :deep(.t-card__title), .dashboard-rank-card :deep(.t-card__title), .dashboard-overview-card :deep(.t-card__title) {
  font: var(--td-font-title-large); font-weight: 400;
}
.dashboard-chart-card :deep(.t-card__body), .dashboard-rank-card :deep(.t-card__body), .dashboard-overview-card :deep(.t-card__body) {
  padding: 0; margin-top: var(--td-comp-margin-l);
}
.card-title-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; width: 100%; }
.dashboard-rank__cell {
  display: inline-flex; align-items: center; justify-content: center;
  width: 24px; height: 24px; border-radius: 50%; font-size: 14px; font-weight: 700;
  color: #fff; background-color: var(--td-gray-color-5);
}
.dashboard-rank__cell--top { background: var(--td-brand-color); }
.overview-panel :deep(.t-card__body) { padding: 0; }
.export-panel { display: flex; flex-direction: column; }
.export-panel :deep(.t-card__body) { margin-top: var(--td-comp-margin-s); }
.inner-card { margin-top: var(--td-comp-margin-s); margin-bottom: var(--td-comp-margin-l); }
.inner-card :deep(.t-card__header) { padding-bottom: 0; }
</style>