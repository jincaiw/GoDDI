<template>
  <div>
    <page-header :title="t('dns.apps.title')" />
    <n-card>
      <template v-if="available">
        <n-data-table :columns="columns" :data="apps" :row-key="(row: DNSApp) => row.id" />
      </template>
      <n-empty v-else :description="t('dns.apps.notAvailable')">
        <template #icon>
          <n-icon><apps-outline /></n-icon>
        </template>
        <template #extra>
          <n-text depth="3" style="font-size: 12px;">{{ t('dns.apps.hint') }}</n-text>
        </template>
      </n-empty>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NIcon, NTag } from 'naive-ui'
import { AppsOutline } from '@vicons/ionicons5'
import PageHeader from '@/components/PageHeader.vue'
import { listApps, type DNSApp } from '@/service/api/goddi/dns'

const { t } = useI18n()
const available = ref(false)
const apps = ref<DNSApp[]>([])

const columns = [
  { title: 'ID', key: 'id' },
  { title: 'Name', key: 'name' },
  { title: 'Version', key: 'version', width: 100 },
  { title: 'Status', key: 'enabled', width: 100, render: (row: DNSApp) => h(NTag, { size: 'small', type: row.enabled ? 'success' : 'default' }, { default: () => (row.enabled ? 'ON' : 'OFF') }) },
]

onMounted(async () => {
  // The apps runtime is a placeholder in this release: the endpoint answers
  // 501, which leaves this page in the graceful "not available" state.
  try {
    const result = await listApps()
    apps.value = result ?? []
    available.value = apps.value.length > 0
  } catch {
    available.value = false
  }
})
</script>
