<template>
  <div>
    <page-header :title="t('dashboard.title')" />
    <n-grid :cols="4" :x-gap="16" :y-gap="16" responsive="screen" item-responsive>
      <n-gi span="4 m:2 l:1">
        <n-card class="metric-card">
          <n-statistic :label="t('dashboard.dnsQueriesToday')">
            <template #prefix>
              <n-icon color="#007aff"><globe-outline /></n-icon>
            </template>
            {{ stats.dnsQueries }}
          </n-statistic>
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card class="metric-card">
          <n-statistic :label="t('dashboard.cacheHitRate')">
            <template #prefix>
              <n-icon color="#af52de"><server-outline /></n-icon>
            </template>
            {{ stats.cacheHitRate }}%
          </n-statistic>
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card class="metric-card">
          <n-statistic :label="t('dashboard.activeLeases')">
            <template #prefix>
              <n-icon color="#ff9500"><desktop-outline /></n-icon>
            </template>
            {{ stats.activeLeases }}
          </n-statistic>
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card class="metric-card">
          <n-statistic :label="t('dashboard.ipamUsage')">
            <template #prefix>
              <n-icon color="#007aff"><grid-outline /></n-icon>
            </template>
            {{ stats.ipamUsage }}%
          </n-statistic>
        </n-card>
      </n-gi>
    </n-grid>

    <n-grid :cols="2" :x-gap="16" :y-gap="16" style="margin-top: 16px;" responsive="screen" item-responsive>
      <n-gi span="2 l:1">
        <n-card :title="t('dashboard.queryChart')">
          <dashboard-chart v-if="chartReady" :hours="chartData.hours" :queries="chartData.queries" />
          <n-skeleton v-else :height="300" />
        </n-card>
      </n-gi>
      <n-gi span="2 l:1">
        <n-card :title="t('dashboard.recentEvents')">
          <n-data-table :columns="eventColumns" :data="(recentEvents as any[])" :bordered="false" size="small" />
        </n-card>
      </n-gi>
    </n-grid>
  </div>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { GlobeOutline, ServerOutline, DesktopOutline, GridOutline } from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import { getDashboardStats, type HourlyDNSStats } from '@/api/dns'
import { listAuditLogs } from '@/api/logs'

const DashboardChart = defineAsyncComponent(() => import('@/components/DashboardChart.vue'))

const { t, locale } = useI18n()

const stats = ref({
  dnsQueries: 0,
  cacheHitRate: 0,
  activeLeases: 0,
  ipamUsage: 0,
})

const chartReady = ref(false)
const chartData = ref<{ hours: string[]; queries: number[] }>({ hours: [], queries: [] })

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

<style scoped>
.metric-card {
  height: 100%;
  min-height: 128px;
  position: relative;
  border-radius: 16px;
}

.metric-card :deep(.n-statistic__label), .metric-card :deep(.n-statistic-value) { margin-left: 56px; }
.metric-card :deep(.n-statistic__label) { font-size: 13px; min-height: 40px; margin-bottom: 0; }
.metric-card :deep(.n-statistic-value__content) { font-size: 28px; font-weight: 600; letter-spacing: -0.8px; }
.metric-card :deep(.n-statistic-value__prefix) { position: absolute; left: 20px; top: 40px; margin: 0; width: 42px; height: 42px; display: grid; place-items: center; border-radius: 50%; background: #007aff0c; }
</style>
