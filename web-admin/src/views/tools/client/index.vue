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
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import { executeDNSQuery, type DNSQueryResponse, type DNSQueryAnswer } from '@/service/api/goddi/dns'

const { t } = useI18n()
const message = useMessage()

const querying = ref(false)
const queryResult = ref<DNSQueryResponse | null>(null)

const queryForm = reactive({
  name: '',
  type: 'A',
  upstream: '',
})

const typeOptions = ['A', 'AAAA', 'CNAME', 'MX', 'NS', 'PTR', 'SOA', 'SRV', 'TXT', 'ANY'].map(t => ({ label: t, value: t }))

const answerColumns = [
  { title: () => t('dns.records.recordName'), key: 'name' },
  { title: () => t('dns.records.recordType'), key: 'type', width: 80 },
  { title: () => t('dns.records.recordValue'), key: 'data' },
  { title: () => t('dns.zones.ttl'), key: 'ttl', width: 80 },
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
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    querying.value = false
  }
}
</script>
