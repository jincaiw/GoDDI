<script setup lang="ts">
import { computed } from 'vue'
import { useThemeStore } from '@/store/modules/theme'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const props = defineProps<{ hours: string[]; queries: number[] }>()
const themeStore = useThemeStore()
const { t } = useI18n()

/** Read a live TDesign custom property, with a token-derived fallback. */
function cssVar(name: string, fallback: string) {
  if (typeof window === 'undefined') return fallback
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback
}

const chartTokens = computed(() => {
  // Touch reactive theme state so CSS variables are re-read after a change.
  void themeStore.darkMode
  void themeStore.themeColor
  void themeStore.otherColor

  const dark = themeStore.darkMode
  return {
    text: cssVar('--td-text-color-secondary', dark ? 'rgba(255, 255, 255, 0.55)' : 'rgba(0, 0, 0, 0.6)'),
    grid: cssVar('--td-border-level-1-color', dark ? '#393939' : '#e8e8e8'),
    // In dark mode the brand is one palette step lighter (palette 400) so the
    // series keeps >=3:1 contrast on TDesign's dark container background.
    series: cssVar(dark ? '--td-brand-color-hover' : '--td-brand-color', dark ? '#2575e6' : '#0052D9')
  }
})

const option = computed(() => ({
  animation: false,
  textStyle: { fontFamily: '-apple-system, BlinkMacSystemFont, sans-serif', color: chartTokens.value.text },
  tooltip: { trigger: 'axis' },
  grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
  xAxis: {
    type: 'category',
    boundaryGap: false,
    data: props.hours,
  },
  yAxis: {
    type: 'value',
    splitLine: { lineStyle: { color: chartTokens.value.grid, type: 'dashed' } }
  },
  series: [
    {
      name: 'DNS Queries',
      type: 'line',
      smooth: true,
      showSymbol: true,
      lineStyle: { color: chartTokens.value.series },
      areaStyle: { color: chartTokens.value.series, opacity: 0.3 },
      itemStyle: { color: chartTokens.value.series },
      data: props.queries,
    },
  ],
}))
</script>

<template>
  <VChart v-if="hours.length" :option="option" style="height: 300px;" autoresize />
  <div v-else style="height: 300px; display: grid; place-items: center;"><TEmpty :description="t('common.noData')" /></div>
</template>
