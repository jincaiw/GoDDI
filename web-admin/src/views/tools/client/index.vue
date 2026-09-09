<template>
  <div>
    <page-header :title="t('dns.client.title')" />

    <n-card>
      <n-form :model="queryForm" label-placement="left" label-width="100px" inline>
        <n-form-item :label="t('dns.client.queryName')">
          <n-input v-model:value="queryForm.name" placeholder="example.com" style="width: 240px;" />
        </n-form-item>
        <n-form-item :label="t('dns.client.queryType')">
          <n-select v-model:value="queryForm.type" :options="typeOptions" style="width: 120px;" />
        </n-form-item>
        <n-form-item :label="t('dns.client.upstream')">
          <n-input v-model:value="queryForm.upstream" placeholder="Optional" style="width: 200px;" clearable />
        </n-form-item>
        <n-form-item>
          <n-button type="primary" :loading="querying" @click="handleQuery">{{ t('dns.client.execute') }}</n-button>
        </n-form-item>
      </n-form>
    </n-card>

    <n-card v-if="queryResult" :title="t('dns.client.response')" style="margin-top: 16px;">
      <n-descriptions :column="2" label-placement="left" style="margin-bottom: 16px;">
        <n-descriptions-item :label="t('dns.client.queryTime')">{{ (queryResult.duration / 1e6).toFixed(2) }}ms</n-descriptions-item>
        <n-descriptions-item :label="t('dns.client.server')">{{ queryResult.upstream }}</n-descriptions-item>
      </n-descriptions>

      <n-data-table
        :columns="answerColumns"
        :data="queryResult.answers || []"
        :bordered="false"
        size="small"
      />
    </n-card>

    <!-- Import answer into a local zone (Technitium DNS Client parity) -->
    <n-modal v-if="showImport" v-model:show="showImport" preset="card" :title="t('dns.client.importToZone')" style="width: 480px;">
      <n-form :model="importForm" label-placement="left" label-width="100px">
        <n-form-item :label="t('dns.zones.title')">
          <n-select
            v-model:value="importForm.zone_id"
            :options="zoneOptions"
            filterable
            :placeholder="t('dns.client.selectZone')"
            style="width: 100%;"
          />
        </n-form-item>
        <n-form-item :label="t('dns.records.recordName')">
          <n-input v-model:value="importForm.name" />
        </n-form-item>
        <n-form-item :label="t('dns.records.recordType')">
          <n-input v-model:value="importForm.type" disabled />
        </n-form-item>
        <n-form-item :label="t('dns.records.recordValue')">
          <n-input v-model:value="importForm.value" type="textarea" :rows="2" />
        </n-form-item>
        <n-form-item :label="t('dns.zones.ttl')">
          <n-input-number v-model:value="importForm.ttl" :min="0" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showImport = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="importing" @click="handleImportSubmit">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSelect, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import {
  executeDNSQuery, listDNSZones, createDNSRecord,
  type DNSQueryResponse, type DNSQueryAnswer, type DNSZone,
} from '@/service/api/goddi/dns'
import { usePermission } from '@/composables/usePermission'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const querying = ref(false)
const queryResult = ref<DNSQueryResponse | null>(null)

const queryForm = reactive({
  name: '',
  type: 'A',
  upstream: '',
})

const typeOptions = ['A', 'AAAA', 'CNAME', 'MX', 'NS', 'PTR', 'SOA', 'SRV', 'TXT', 'ANY'].map(t => ({ label: t, value: t }))

const showImport = ref(false)
const importing = ref(false)
const zoneOptions = ref<{ label: string; value: string }[]>([])
const importForm = reactive({ zone_id: '', name: '', type: 'A', value: '', ttl: 3600 })

const answerColumns = [
  { title: () => t('dns.records.recordName'), key: 'name' },
  { title: () => t('dns.records.recordType'), key: 'type', width: 80 },
  { title: () => t('dns.records.recordValue'), key: 'data' },
  { title: () => t('dns.zones.ttl'), key: 'ttl', width: 80 },
  {
    title: () => t('common.actions'),
    key: 'actions',
    width: 90,
    render: (row: DNSQueryAnswer) => h(
      NButton,
      {
        size: 'small',
        text: true,
        type: 'primary',
        disabled: !perm.canWrite('dns'),
        onClick: () => openImport(row),
      },
      { default: () => t('dns.client.importToZone') },
    ),
  },
]

async function handleQuery() {
  if (!queryForm.name) {
    message.warning('Please enter a domain name')
    return
  }
  querying.value = true
  try {
    queryResult.value = await executeDNSQuery({
      name: queryForm.name,
      type: queryForm.type,
      upstream: queryForm.upstream || undefined,
    })
  } catch (err: unknown) {
    // Drop any previously displayed result so the failure isn't shown next
    // to stale answers from a prior successful query.
    queryResult.value = null
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    querying.value = false
  }
}

async function openImport(row: DNSQueryAnswer) {
  importForm.name = row.name
  importForm.type = row.type
  importForm.value = row.data
  importForm.ttl = row.ttl || 3600
  showImport.value = true
  // Lazy-load the zone picker once per modal session.
  if (zoneOptions.value.length === 0) {
    try {
      const result = await listDNSZones({ page: 1, page_size: 100 })
      zoneOptions.value = (result.data ?? [])
        .filter((z: DNSZone) => z.type !== 'allowed' && z.type !== 'blocked')
        .map((z: DNSZone) => ({ label: z.name, value: z.id }))
    } catch (err: unknown) {
      message.error(err instanceof Error ? err.message : t('common.failed'))
    }
  }
}

async function handleImportSubmit() {
  if (!importForm.zone_id || !importForm.name || !importForm.value) {
    message.warning(t('common.required'))
    return
  }
  importing.value = true
  try {
    await createDNSRecord(importForm.zone_id, {
      name: importForm.name,
      type: importForm.type,
      value: importForm.value,
      ttl: importForm.ttl,
    })
    message.success(t('common.createSuccess'))
    showImport.value = false
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    importing.value = false
  }
}
</script>
