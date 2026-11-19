<script setup lang="ts">
import { computed } from 'vue';
import { useThemeStore } from '@/store/modules/theme';
import VChart from 'vue-echarts';
import { use } from 'echarts/core';
import { BarChart, LineChart } from 'echarts/charts';
import { GridComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';

use([LineChart, BarChart, GridComponent, CanvasRenderer]);

const props = withDefaults(defineProps<{ values: number[]; type?: 'line' | 'bar'; inverse?: boolean }>(), {
  type: 'line',
  inverse: false
});

const themeStore = useThemeStore();

function cssVar(name: string, fallback: string) {
  if (typeof window === 'undefined') return fallback;
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}

const hasData = computed(() => props.values.some(value => Number(value) > 0));

const option = computed(() => {
  void themeStore.darkMode;
  void themeStore.themeColor;
  const color = props.inverse
    ? cssVar('--td-text-color-anti', '#ffffff')
    : cssVar('--td-brand-color', '#0052D9');
  const data = props.values;
  return {
    animation: false,
    grid: { left: 0, right: 0, top: 6, bottom: 0 },
    xAxis: { type: 'category', show: false, data: data.map((_, index) => index) },
    yAxis: { type: 'value', show: false, scale: true },
    series: [
      props.type === 'bar'
        ? {
            type: 'bar',
            data,
            barWidth: 8,
            itemStyle: { color, borderRadius: [4, 4, 0, 0] }
          }
        : {
            type: 'line',
            data,
            smooth: true,
            symbol: 'none',
            lineStyle: { color, width: 2 },
            areaStyle: { color, opacity: props.inverse ? 0.18 : 0.24 }
          }
    ]
  };
});
</script>

<template>
  <div v-if="hasData" class="sparkline">
    <VChart :option="option" autoresize />
  </div>
  <div v-else class="sparkline sparkline--empty" aria-hidden="true" />
</template>

<style scoped>
.sparkline {
  width: 120px;
  height: 56px;
}
.sparkline--empty {
  border-bottom: 1px dashed var(--td-border-level-1-color);
}
</style>
