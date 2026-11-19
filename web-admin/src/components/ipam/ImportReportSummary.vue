<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { ImportReport } from '@/service/api/goddi/ipam';

defineOptions({ name: 'IPAMImportReportSummary' });

defineProps<{
  report: ImportReport;
}>();

const { t } = useI18n();
</script>

<template>
  <TDescriptions :column="4" bordered size="small" class="mb-4">
    <TDescriptionsItem :label="t('ipam.import.total')">{{ report.total }}</TDescriptionsItem>
    <TDescriptionsItem :label="t('ipam.import.creates')">
      <span :class="{ bold: report.creates > 0 }">{{ report.creates }}</span>
    </TDescriptionsItem>
    <TDescriptionsItem :label="t('ipam.import.updates')">
      <span :class="{ bold: report.updates > 0 }">{{ report.updates }}</span>
    </TDescriptionsItem>
    <TDescriptionsItem :label="t('ipam.import.unchanged')">{{ report.unchanged }}</TDescriptionsItem>
  </TDescriptions>
</template>

<style scoped>
.mb-4 {
  margin-bottom: 16px;
}
.bold {
  font-weight: 600;
}
</style>
