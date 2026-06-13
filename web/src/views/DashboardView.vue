<template>
  <div>
    <page-header :title="t('dashboard.title')" />
    <n-grid :cols="4" :x-gap="16" :y-gap="16" responsive="screen" item-responsive>
      <n-gi span="4 m:2 l:1">
        <n-card class="metric-card">
          <n-statistic :label="t('dashboard.dnsQueriesToday')">
            <template #prefix>
              <n-icon color="#18a058"><globe-outline /></n-icon>
            </template>
            {{ stats.dnsQueries }}
          </n-statistic>
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card class="metric-card">
          <n-statistic :label="t('dashboard.cacheHitRate')">
            <template #prefix>
              <n-icon color="#2080f0"><server-outline /></n-icon>
            </template>
            {{ stats.cacheHitRate }}%
          </n-statistic>
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card class="metric-card">
          <n-statistic :label="t('dashboard.activeLeases')">
            <template #prefix>
              <n-icon color="#f0a020"><desktop-outline /></n-icon>
            </template>
            {{ stats.activeLeases }}
          </n-statistic>
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card class="metric-card">
          <n-statistic :label="t('dashboard.ipamUsage')">
            <template #prefix>
              <n-icon color="#d03050"><grid-outline /></n-icon>
            </template>
            {{ stats.ipamUsage }}%
          </n-statistic>
        </n-card>
      </n-gi>
    </n-grid>

    <n-grid :cols="2" :x-gap="16" :y-gap="16" style="margin-top: 16px;" responsive="screen" item-responsive>
      <n-gi span="2 l:1">
        <n-card :title="t('dashboard.queryChart')">
          <v-chart :option="chartOption" style="height: 300px;" autoresize />
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
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { GlobeOutline, ServerOutline, DesktopOutline, GridOutline } from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import { getDashboardStats, type HourlyDNSStats } from '@/api/dns'
import { listAuditLogs } from '@/api/logs'

use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const { t } = useI18n()

const stats = ref({
  dnsQueries: 0,
  cacheHitRate: 0,
  activeLeases: 0,
  ipamUsage: 0,
})

const chartData = ref<{ hours: string[]; queries: number[] }>({ hours: [], queries: [] })

const chartOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
  xAxis: {
    type: 'category',
    boundaryGap: false,
    data: chartData.value.hours,
  },
  yAxis: { type: 'value' },
  series: [
    {
      name: 'DNS Queries',
      type: 'line',
      smooth: true,
      areaStyle: { opacity: 0.3 },
      data: chartData.value.queries,
      itemStyle: { color: '#18a058' },
    },
  ],
}))

const eventColumns = [
  { title: t('logs.audit.user'), key: 'username', width: 100 },
  { title: t('logs.audit.action'), key: 'action', width: 100 },
  { title: t('logs.audit.resource'), key: 'resource_type', width: 120 },
  { title: t('common.createdAt'), key: 'created_at', width: 160 },
]

const recentEvents = ref<unknown[]>([])

onMounted(async () => {
  try {
    // Load dashboard data and audit logs in parallel
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

      // Use real hourly data from backend
      const hourlyData = d.recent_dns_stats as HourlyDNSStats[]
      if (hourlyData && hourlyData.length > 0) {
        const hours = hourlyData.map(item => {
          // Backend currently returns "YYYY-MM-DD HH:MM:SS"; display HH:MM only.
          // The space-split keeps us tolerant to small format changes (e.g.
          // "HH:MM" alone or "YYYY-MM-DDTHH:MM:SSZ").
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
  }
})
</script>

<style scoped>
.metric-card {
  height: 100%;
  border-radius: 12px;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.metric-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08);
}
</style>
