<template>
  <v-chart v-if="hours.length" :option="option" style="height: 300px;" autoresize />
  <div v-else style="height: 300px; display: grid; place-items: center;"><n-empty :description="t('common.noData')" /></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useAppStore } from '@/stores/app'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const props = defineProps<{ hours: string[]; queries: number[] }>()
const appStore = useAppStore()
const { t } = useI18n()

const option = computed(() => ({
  animation: false,
  textStyle: { fontFamily: '-apple-system, BlinkMacSystemFont, sans-serif', color: appStore.darkMode ? '#a1a1a6' : '#6e6e73' },
  tooltip: { trigger: 'axis' },
  grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
  xAxis: {
    type: 'category',
    boundaryGap: false,
    data: props.hours,
  },
  yAxis: { type: 'value', splitLine: { lineStyle: { color: appStore.darkMode ? '#3a3a3c' : '#e5e5ea', type: 'dashed' } } },
  series: [
    {
      name: 'DNS Queries',
      type: 'line',
      smooth: true,
      showSymbol: true,
      areaStyle: { opacity: 0.3 },
      data: props.queries,
      itemStyle: { color: '#0a84ff' },
    },
  ],
}))
</script>
