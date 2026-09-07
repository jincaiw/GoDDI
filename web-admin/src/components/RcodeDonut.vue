<template>
  <v-chart v-if="total > 0" :option="option" style="height: 300px;" autoresize />
  <div v-else style="height: 300px; display: grid; place-items: center;"><n-empty :description="t('common.noData')" /></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useThemeStore } from '@/store/modules/theme'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { PieChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
use([PieChart, TooltipComponent, LegendComponent, CanvasRenderer])

export interface RcodeSummary {
  total: number
  noerror: number
  nxdomain: number
  servfail: number
  refused: number
  other: number
}

const props = defineProps<{ rcodes: RcodeSummary }>()
const appStore = useThemeStore()
const { t } = useI18n()

const total = computed(() => props.rcodes?.total ?? 0)

const option = computed(() => ({
  animation: false,
  textStyle: { fontFamily: '-apple-system, BlinkMacSystemFont, sans-serif', color: appStore.darkMode ? '#a1a1a6' : '#6e6e73' },
  tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
  legend: { bottom: 0, icon: 'circle', itemWidth: 8, itemHeight: 8 },
  series: [
    {
      name: t('dashboard.rcodes'),
      type: 'pie',
      radius: ['45%', '70%'],
      center: ['50%', '45%'],
      avoidLabelOverlap: true,
      itemStyle: { borderRadius: 4, borderColor: appStore.darkMode ? '#1c1c1e' : '#fff', borderWidth: 2 },
      label: { show: false },
      data: [
        { name: 'NOERROR', value: props.rcodes.noerror, itemStyle: { color: '#34c759' } },
        { name: 'NXDOMAIN', value: props.rcodes.nxdomain, itemStyle: { color: '#ff9500' } },
        { name: 'SERVFAIL', value: props.rcodes.servfail, itemStyle: { color: '#ff3b30' } },
        { name: 'REFUSED', value: props.rcodes.refused, itemStyle: { color: '#af52de' } },
        { name: t('dashboard.rcodesOther'), value: props.rcodes.other, itemStyle: { color: '#8e8e93' } },
      ].filter(d => d.value > 0),
    },
  ],
}))
</script>
