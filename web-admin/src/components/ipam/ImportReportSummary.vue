<template>
  <n-descriptions :column="4" bordered size="small" label-placement="top" style="margin-bottom: 16px;">
    <n-descriptions-item :label="t('ipam.import.total')">{{ report.total }}</n-descriptions-item>
    <n-descriptions-item :label="t('ipam.import.creates')">
      <span :style="report.creates > 0 ? 'font-weight:500' : ''">{{ report.creates }}</span>
    </n-descriptions-item>
    <n-descriptions-item :label="t('ipam.import.updates')">
      <span :style="report.updates > 0 ? 'font-weight:500' : ''">{{ report.updates }}</span>
    </n-descriptions-item>
    <n-descriptions-item :label="t('ipam.import.unchanged')">{{ report.unchanged }}</n-descriptions-item>
  </n-descriptions>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { ImportReport } from '@/service/api/goddi/ipam'

defineProps<{
  report: ImportReport
}>()

const { t } = useI18n()
</script>
