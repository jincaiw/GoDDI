<script setup lang="ts">
import { computed } from 'vue';

/**
 * Deployment-mode badge.
 *
 * SINGLE-NODE-FIRST: the console currently always renders `single`. The props
 * already model the HA modes so that stage 3 (dual-node / HA) can bind this to
 * a real /api/v1/cluster/status without changing any page or the layout API.
 * No store field, request or HA state is introduced in the single-node phase.
 * See docs/tdesign-migration.md §3.
 */
type NodeMode = 'single' | 'ha-primary' | 'ha-standby' | 'ha-degraded';

const props = withDefaults(defineProps<{ mode?: NodeMode }>(), { mode: 'single' });

const MAP: Record<NodeMode, { label: string; theme: 'success' | 'primary' | 'warning' | 'danger' }> = {
  single: { label: 'Single-Node', theme: 'success' },
  'ha-primary': { label: 'HA · Primary', theme: 'primary' },
  'ha-standby': { label: 'HA · Standby', theme: 'warning' },
  'ha-degraded': { label: 'HA · Degraded', theme: 'danger' }
};

const view = computed(() => MAP[props.mode]);
</script>

<template>
  <TTag :theme="view.theme" variant="light-outline">{{ view.label }}</TTag>
</template>
