<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import VChart from 'vue-echarts';
import { use } from 'echarts/core';
import { LineChart } from 'echarts/charts';
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import { useThemeStore } from '@/store/modules/theme';
import type { StatsResponse } from '@/service/api/goddi/dns';

use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

const props = defineProps<{ stats: StatsResponse | null }>();
const themeStore = useThemeStore();
const { t } = useI18n();

function cssVar(name: string, fallback: string) {
  if (typeof window === 'undefined') return fallback;
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}

const hasData = computed(() => (props.stats?.series?.length ?? 0) > 0);

const tokens = computed(() => {
  void themeStore.darkMode;
  void themeStore.themeColor;
  void themeStore.otherColor;
  const dark = themeStore.darkMode;
  return {
    text: cssVar('--td-text-color-secondary', dark ? 'rgba(255,255,255,.55)' : 'rgba(0,0,0,.6)'),
    grid: cssVar('--td-border-level-1-color', dark ? '#393939' : '#e8e8e8'),
    brand: cssVar(dark ? '--td-brand-color-hover' : '--td-brand-color', dark ? '#2575e6' : '#0052D9'),
    success: cssVar('--td-success-color', '#00A870')
  };
});

const option = computed(() => {
  const s = props.stats;
  if (!s) return {};
  return {
    animation: false,
    textStyle: { color: tokens.value.text },
    color: [tokens.value.brand, tokens.value.success],
    tooltip: { trigger: 'axis' },
    legend: { bottom: 0, icon: 'circle', itemWidth: 8, itemHeight: 8 },
    grid: { left: 48, right: 24, top: 24, bottom: 48 },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: s.series.map(point => point.bucket),
      axisLine: { lineStyle: { color: tokens.value.grid } }
    },
    yAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: tokens.value.grid, type: 'dashed' } }
    },
    series: [
      {
        name: t('dashboard.statsTotal'),
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: s.series.map(point => point.total),
        areaStyle: { opacity: 0.18 }
      },
      {
        name: t('dashboard.statsCached'),
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: s.series.map(point => point.cached)
      }
    ]
  };
});
</script>

<template>
  <VChart v-if="hasData" :option="option" style="height: 320px" autoresize />
  <div v-else style="height: 320px; display: grid; place-items: center">
    <TEmpty :description="t('common.noData')" />
  </div>
</template>
