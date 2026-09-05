<template>
  <v-chart :option="option" style="height: 300px;" autoresize />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const props = defineProps<{ hours: string[]; queries: number[] }>()

const option = computed(() => ({
  animation: false,
  tooltip: { trigger: 'axis' },
  grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
  xAxis: {
    type: 'category',
    boundaryGap: false,
    data: props.hours,
  },
  yAxis: { type: 'value' },
  series: [
    {
      name: 'DNS Queries',
      type: 'line',
      smooth: true,
      showSymbol: true,
      areaStyle: { opacity: 0.3 },
      data: props.queries,
      itemStyle: { color: '#18a058' },
    },
  ],
}))
</script>
