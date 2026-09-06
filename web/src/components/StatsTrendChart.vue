<template>
  <v-chart v-if="hasData" :option="option" style="height: 300px;" autoresize />
  <div v-else style="height: 300px; display: grid; place-items: center;"><n-empty :description="t('common.noData')" /></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useAppStore } from '@/stores/app'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent, GridComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
use([BarChart, LineChart, TooltipComponent, LegendComponent, GridComponent, CanvasRenderer])

import type { StatsResponse } from '@/api/dns'

const props = defineProps<{ stats: StatsResponse | null }>()
const appStore = useAppStore()
const { t } = useI18n()

const hasData = computed(() => (props.stats?.series?.length ?? 0) > 0)

const option = computed(() => {
  const s = props.stats
  if (!s) return {}
  const buckets = s.series.map(p => p.bucket)
  return {
    animation: false,
    textStyle: { fontFamily: '-apple-system, BlinkMacSystemFont, sans-serif', color: appStore.darkMode ? '#a1a1a6' : '#6e6e73' },
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    legend: { bottom: 0, icon: 'circle', itemWidth: 8, itemHeight: 8 },
    grid: { left: 48, right: 16, top: 24, bottom: 48 },
    xAxis: { type: 'category', data: buckets, axisLabel: { hideOverlap: true } },
    yAxis: { type: 'value', splitLine: { lineStyle: { opacity: 0.25 } } },
    series: [
      {
        name: t('dashboard.statsTotal'),
        type: 'bar',
        stack: 'total',
        barMaxWidth: 28,
        itemStyle: { color: '#007aff' },
        data: s.series.map(p => p.total - p.blocked - p.cached),
      },
      {
        name: t('dashboard.statsCached'),
        type: 'bar',
        stack: 'total',
        itemStyle: { color: '#34c759' },
        data: s.series.map(p => p.cached),
      },
      {
        name: t('dashboard.statsBlocked'),
        type: 'bar',
        stack: 'total',
        itemStyle: { color: '#ff3b30' },
        data: s.series.map(p => p.blocked),
      },
    ],
  }
})
</script>
