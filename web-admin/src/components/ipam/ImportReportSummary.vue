<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { ImportReport } from '@/service/api/goddi/ipam'

defineProps<{
  report: ImportReport
}>()

const { t } = useI18n()
</script>

<template>
  <NDescriptions :column="4" bordered size="small" label-placement="top" style="margin-bottom: 16px;">
    <NDescriptionsItem :label="t('ipam.import.total')">{{ report.total }}</NDescriptionsItem>
    <NDescriptionsItem :label="t('ipam.import.creates')">
      <span :style="report.creates > 0 ? 'font-weight:500' : ''">{{ report.creates }}</span>
    </NDescriptionsItem>
    <NDescriptionsItem :label="t('ipam.import.updates')">
      <span :style="report.updates > 0 ? 'font-weight:500' : ''">{{ report.updates }}</span>
    </NDescriptionsItem>
    <NDescriptionsItem :label="t('ipam.import.unchanged')">{{ report.unchanged }}</NDescriptionsItem>
  </NDescriptions>
</template>
