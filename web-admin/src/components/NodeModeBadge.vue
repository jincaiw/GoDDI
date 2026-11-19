<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useNodeMode, type NodeMode } from '@/composables/useNodeMode';

/**
 * Deployment-mode badge. The mode comes from the live read-only HA status,
 * not from a build-time constant. A 'mode' prop is kept only for stories and
 * tests that need to force a state; production callers pass nothing.
 */
const props = defineProps<{ mode?: NodeMode }>();
const { t } = useI18n();
const { mode: liveMode } = useNodeMode();

const current = computed<NodeMode>(() => props.mode ?? liveMode.value);

const theme = computed(() => {
  switch (current.value) {
    case 'single': return 'success';
    case 'ha-primary': return 'primary';
    case 'ha-standby': return 'warning';
    default: return 'danger';
  }
});

const label = computed(() => {
  switch (current.value) {
    case 'single': return t('nodeMode.single');
    case 'ha-primary': return t('nodeMode.primary');
    case 'ha-standby': return t('nodeMode.standby');
    default: return t('nodeMode.degraded');
  }
});
</script>

<template>
  <TTag :theme="theme" variant="light-outline">{{ label }}</TTag>
</template>
