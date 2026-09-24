<script setup lang="ts">
import { computed, defineAsyncComponent, ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { GlobeOutline, ServerOutline, DesktopOutline, GridOutline } from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import { getDashboardStats, getDashboardTop, getDNSStats, type HourlyDNSStats, type TopStats, type StatsResponse } from '@/service/api/goddi/dns'
import { listAuditLogs } from '@/service/api/goddi/logs'

const DashboardChart = defineAsyncComponent(() => import('@/components/DashboardChart.vue'))
const StatsTrendChart = defineAsyncComponent(() => import('@/components/StatsTrendChart.vue'))

const { t, locale } = useI18n()

const stats = ref({
  dnsQueries: 0,
  cacheHitRate: 0,
  activeLeases: 0,
  ipamUsage: 0,
})

const chartReady = ref(false)
const chartData = ref<{ hours: string[]; queries: number[] }>({ hours: [], queries: [] })

// Top statistics
const topRange = ref<'hour' | 'day' | 'week'>('hour')
const topStats = ref<TopStats>({ range: 'hour', top_clients: [], top_domains: [], top_blocked: [], rcodes: { total: 0, noerror: 0, nxdomain: 0, servfail: 0, refused: 0, other: 0 } })

async function loadTop() {
  try { topStats.value = await getDashboardTop(topRange.value, 10) } catch { /* ignore */ }
}

// Long-term statistics (GET /stats, Technitium parity A2).
const statsRange = ref<'hour' | 'day' | 'week' | 'month' | 'year'>('day')
const ltStats = ref<StatsResponse | null>(null)

async function loadLongTermStats() {
  try { ltStats.value = await getDNSStats({ range: statsRange.value }) } catch { /* ignore */ }
}

const eventColumns = computed(() => [
  { title: () => t('logs.audit.user'), key: 'username', width: 100 },
  { title: () => t('logs.audit.action'), key: 'action', width: 100 },
  { title: () => t('logs.audit.resource'), key: 'resource_type', width: 120 },
  { title: () => t('common.createdAt'), key: 'created_at', width: 160,
    render: (row: { created_at: string }) => {
      const date = new Date(row.created_at)
      return Number.isNaN(date.getTime()) ? row.created_at : date.toLocaleString(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
    },
  },
])

const recentEvents = ref<unknown[]>([])

onMounted(async () => {
  loadTop()
  loadLongTermStats()
  try {
    const [dashboardResult, auditResult] = await Promise.allSettled([
      getDashboardStats(),
      listAuditLogs({ page: 1, page_size: 10 }),
    ])

    if (dashboardResult.status === 'fulfilled') {
      const d = dashboardResult.value
      stats.value.dnsQueries = d.dns_queries_today
      stats.value.cacheHitRate = Math.round(d.cache_hit_rate)
      stats.value.activeLeases = d.active_leases
      stats.value.ipamUsage = Math.round(d.ipam_usage)

      const hourlyData = d.recent_dns_stats as HourlyDNSStats[]
      if (hourlyData && hourlyData.length > 0) {
        const hours = hourlyData.map(item => {
          const parts = item.hour.split(' ')
          const timePart = parts[1] ?? item.hour
          return timePart.substring(0, 5)
        })
        const queries = hourlyData.map(item => item.queries)
        chartData.value = { hours, queries }
      }
    }

    if (auditResult.status === 'fulfilled') {
      recentEvents.value = auditResult.value.data
    }
  } catch {
    // Silently handle dashboard errors
  } finally {
    chartReady.value = true
  }
})
</script>

<template>
  <div>
    <PageHeader :title="t('dashboard.title')" />
    <NGrid :cols="4" :x-gap="16" :y-gap="16" responsive="screen" item-responsive>
      <NGi span="4 m:2 l:1">
        <NCard class="metric-card">
          <NStatistic :label="t('dashboard.dnsQueriesToday')">
            <template #prefix>
              <NIcon color="#0a84ff"><GlobeOutline /></NIcon>
            </template>
            {{ stats.dnsQueries.toLocaleString('en-US') }}
          </NStatistic>
        </NCard>
      </NGi>
      <NGi span="4 m:2 l:1">
        <NCard class="metric-card">
          <NStatistic :label="t('dashboard.cacheHitRate')">
            <template #prefix>
              <NIcon color="#af52de"><ServerOutline /></NIcon>
            </template>
            {{ stats.cacheHitRate }}%
          </NStatistic>
        </NCard>
      </NGi>
      <NGi span="4 m:2 l:1">
        <NCard class="metric-card">
          <NStatistic :label="t('dashboard.activeLeases')">
            <template #prefix>
              <NIcon color="#ff9500"><DesktopOutline /></NIcon>
            </template>
            {{ stats.activeLeases }}
          </NStatistic>
        </NCard>
      </NGi>
      <NGi span="4 m:2 l:1">
        <NCard class="metric-card">
          <NStatistic :label="t('dashboard.ipamUsage')">
            <template #prefix>
              <NIcon color="#0a84ff"><GridOutline /></NIcon>
            </template>
            {{ stats.ipamUsage }}%
          </NStatistic>
        </NCard>
      </NGi>
    </NGrid>

    <NCard style="margin-top: 16px;" :title="t('dashboard.topStats')">
      <template #header-extra>
        <NRadioGroup v-model:value="topRange" size="small" @update:value="loadTop">
          <NRadioButton value="hour">{{ t('dashboard.rangeHour') }}</NRadioButton>
          <NRadioButton value="day">{{ t('dashboard.rangeDay') }}</NRadioButton>
          <NRadioButton value="week">{{ t('dashboard.rangeWeek') }}</NRadioButton>
        </NRadioGroup>
      </template>
      <NGrid :cols="3" :x-gap="16" :y-gap="16" responsive="screen" item-responsive>
        <NGi span="3 l:1">
          <h4 class="top-title">{{ t('dashboard.topClients') }}</h4>
          <NEmpty v-if="topStats.top_clients.length === 0" size="small" :description="t('common.noData')" />
          <ul v-else class="top-list">
            <li v-for="e in topStats.top_clients" :key="e.name">
              <span class="top-name">{{ e.name }}</span>
              <span class="top-count">{{ e.count }}</span>
            </li>
          </ul>
        </NGi>
        <NGi span="3 l:1">
          <h4 class="top-title">{{ t('dashboard.topDomains') }}</h4>
          <NEmpty v-if="topStats.top_domains.length === 0" size="small" :description="t('common.noData')" />
          <ul v-else class="top-list">
            <li v-for="e in topStats.top_domains" :key="e.name">
              <span class="top-name">{{ e.name }}</span>
              <span class="top-count">{{ e.count }}</span>
            </li>
          </ul>
        </NGi>
        <NGi span="3 l:1">
          <h4 class="top-title">{{ t('dashboard.topBlocked') }}</h4>
          <NEmpty v-if="topStats.top_blocked.length === 0" size="small" :description="t('common.noData')" />
          <ul v-else class="top-list">
            <li v-for="e in topStats.top_blocked" :key="e.name">
              <span class="top-name">{{ e.name }}</span>
              <span class="top-count">{{ e.count }}</span>
            </li>
          </ul>
        </NGi>
      </NGrid>
    </NCard>

    <NCard style="margin-top: 16px;" :title="t('dashboard.longTermStats')">
      <template #header-extra>
        <NRadioGroup v-model:value="statsRange" size="small" @update:value="loadLongTermStats">
          <NRadioButton value="hour">{{ t('dashboard.rangeHour') }}</NRadioButton>
          <NRadioButton value="day">{{ t('dashboard.rangeDay') }}</NRadioButton>
          <NRadioButton value="week">{{ t('dashboard.rangeWeek') }}</NRadioButton>
          <NRadioButton value="month">{{ t('dashboard.rangeMonth') }}</NRadioButton>
          <NRadioButton value="year">{{ t('dashboard.rangeYear') }}</NRadioButton>
        </NRadioGroup>
      </template>
      <NSpace v-if="ltStats" :size="24" style="margin-bottom: 8px;" :wrap="true">
        <NStatistic :label="t('dashboard.statsTotal')" :value="ltStats.summary.total" />
        <NStatistic :label="t('dashboard.statsNoError')" :value="ltStats.summary.noerror" />
        <NStatistic :label="t('dashboard.statsNxDomain')" :value="ltStats.summary.nxdomain" />
        <NStatistic :label="t('dashboard.statsServFail')" :value="ltStats.summary.servfail" />
        <NStatistic :label="t('dashboard.statsRefused')" :value="ltStats.summary.refused" />
        <NStatistic :label="t('dashboard.statsBlocked')" :value="ltStats.summary.blocked" />
        <NStatistic :label="t('dashboard.statsCached')" :value="ltStats.summary.cached" />
        <NStatistic :label="t('dashboard.statsClients')" :value="ltStats.summary.clients" />
        <NStatistic :label="t('dashboard.statsAvgLatency')" :value="ltStats.summary.avg_response_ms.toFixed(1) + 'ms'" />
      </NSpace>
      <StatsTrendChart :stats="ltStats" />
    </NCard>

    <NGrid :cols="2" :x-gap="16" :y-gap="16" style="margin-top: 16px;" responsive="screen" item-responsive>
      <NGi span="2 l:1">
        <NCard :title="t('dashboard.queryChart')">
          <DashboardChart v-if="chartReady" :hours="chartData.hours" :queries="chartData.queries" />
          <NSkeleton v-else :height="300" />
        </NCard>
      </NGi>
      <NGi span="2 l:1">
        <NCard :title="t('dashboard.recentEvents')">
          <NDataTable :columns="eventColumns" :data="(recentEvents as any[])" :bordered="false" size="small" />
        </NCard>
      </NGi>
    </NGrid>
  </div>
</template>

<style scoped>
.metric-card {
  height: 100%;
  min-height: 128px;
  position: relative;
  border-radius: 10px;
}

.metric-card :deep(.n-statistic__label), .metric-card :deep(.n-statistic-value) { margin-left: 56px; }
.metric-card :deep(.n-statistic__label) { font-size: 13px; min-height: 40px; margin-bottom: 0; }
.metric-card :deep(.n-statistic-value__content) { font-size: 28px; font-weight: 600; letter-spacing: -0.8px; }
.metric-card :deep(.n-statistic-value__prefix) { position: absolute; left: 20px; top: 40px; margin: 0; width: 42px; height: 42px; display: grid; place-items: center; border-radius: 50%; background: #0a84ff10; }

.top-title { margin: 0 0 8px; font-size: 13px; color: #888; }
.top-list { list-style: none; margin: 0; padding: 0; }
.top-list li { display: flex; justify-content: space-between; padding: 4px 0; font-size: 13px; border-bottom: 1px dashed rgba(128, 128, 128, 0.15); }
.top-list li:last-child { border-bottom: none; }
.top-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 75%; }
.top-count { font-weight: 600; }
</style>
