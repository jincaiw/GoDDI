<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Link as TLink, MessagePlugin } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import {
  createDNSRecord,
  executeDNSQuery,
  listDNSZones,
  type DNSQueryAnswer,
  type DNSQueryResponse,
  type DNSZone
} from '@/service/api/goddi/dns';
import { usePermission } from '@/composables/usePermission';

defineOptions({ name: 'DNSClientPage' });

/** Starter tool page: TCard sections + TForm query + TTable answers + TDialog import. */

const { t } = useI18n();
const perm = usePermission();

const querying = ref(false);
const queryResult = ref<DNSQueryResponse | null>(null);

const queryForm = reactive({ name: '', type: 'A', upstream: '' });
const typeOptions = ['A', 'AAAA', 'CNAME', 'MX', 'NS', 'PTR', 'SOA', 'SRV', 'TXT', 'ANY'].map(v => ({ label: v, value: v }));

const showImport = ref(false);
const importing = ref(false);
const importFormRef = ref<FormInstanceFunctions | null>(null);
const zoneOptions = ref<Array<{ label: string; value: string }>>([]);
const importForm = reactive({ zone_id: '', name: '', type: 'A', value: '', ttl: 3600 });

const importRules = {
  zone_id: [{ required: true, message: t('dns.zones.title') + t('common.required'), type: 'error' }],
  name: [{ required: true, message: t('dns.records.recordName') + t('common.required'), type: 'error' }],
  value: [{ required: true, message: t('dns.records.recordValue') + t('common.required'), type: 'error' }]
};

const answerColumns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('dns.records.recordName'), ellipsis: true },
  { colKey: 'type', title: t('dns.records.recordType'), width: 100 },
  { colKey: 'data', title: t('dns.records.recordValue'), ellipsis: true },
  { colKey: 'ttl', title: t('dns.zones.ttl'), width: 90 },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 110,
    cell: (h, { row }) =>
      h(
        TLink,
        { theme: 'primary', disabled: !perm.canWrite('dns'), onClick: () => void openImport(row as DNSQueryAnswer) },
        () => t('dns.client.importToZone')
      )
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

async function handleQuery() {
  if (!perm.canRead('dns')) return;
  if (!queryForm.name) {
    void MessagePlugin.warning(t('common.enterDomain'));
    return;
  }
  querying.value = true;
  try {
    queryResult.value = await executeDNSQuery({
      name: queryForm.name,
      type: queryForm.type,
      upstream: queryForm.upstream || undefined
    });
  } catch (err) {
    // Drop stale results so a failure is not shown beside an old answer.
    queryResult.value = null;
    msgError(err);
  } finally {
    querying.value = false;
  }
}

async function openImport(row: DNSQueryAnswer) {
  if (!perm.canWrite('dns')) return;
  Object.assign(importForm, {
    zone_id: '',
    name: row.name,
    type: row.type,
    value: row.data,
    ttl: row.ttl || 3600
  });
  showImport.value = true;
  if (zoneOptions.value.length === 0) {
    try {
      const result = await listDNSZones({ page: 1, page_size: 100 });
      zoneOptions.value = (result.data ?? [])
        .filter((z: DNSZone) => z.type !== 'allowed' && z.type !== 'blocked')
        .map((z: DNSZone) => ({ label: z.name, value: z.id }));
    } catch (err) {
      msgError(err);
    }
  }
}

async function handleImportSubmit() {
  if (!perm.canWrite('dns')) return;
  const valid = await importFormRef.value?.validate();
  if (valid !== true) return;

  importing.value = true;
  try {
    await createDNSRecord(importForm.zone_id, {
      name: importForm.name,
      type: importForm.type,
      value: importForm.value,
      ttl: importForm.ttl
    });
    void MessagePlugin.success(t('common.createSuccess'));
    showImport.value = false;
  } catch (err) {
    msgError(err);
  } finally {
    importing.value = false;
  }
}
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <div class="td-page-title">
        <h2>{{ t('dns.client.title') }}</h2>
      </div>
    </TCard>

    <TCard :bordered="false">
      <TForm :data="queryForm" layout="inline" @submit="handleQuery">
        <TFormItem :label="t('dns.client.queryName')">
          <TInput v-model="queryForm.name" :placeholder="t('common.queryExample')" style="width: 240px" @enter="handleQuery" />
        </TFormItem>
        <TFormItem :label="t('dns.client.queryType')">
          <TSelect v-model="queryForm.type" :options="typeOptions" style="width: 130px" />
        </TFormItem>
        <TFormItem :label="t('dns.client.upstream')">
          <TInput v-model="queryForm.upstream" :placeholder="t('dns.client.upstreamHint')" clearable style="width: 240px" />
        </TFormItem>
        <TFormItem>
          <TButton theme="primary" type="submit" :loading="querying" :disabled="!perm.canRead('dns')">
            {{ t('dns.client.execute') }}
          </TButton>
        </TFormItem>
      </TForm>
    </TCard>

    <TCard v-if="queryResult" :bordered="false" :title="t('dns.client.response')">
      <TAlert v-if="queryResult.source === 'local-authoritative'" theme="success" :message="t('dns.client.localResult')" class="result-alert" />
      <TAlert v-else-if="queryResult.source === 'upstream'" theme="warning" :message="t('dns.client.externalResult')" class="result-alert" />
      <TDescriptions :column="2" class="result-meta">
        <TDescriptionsItem :label="t('dns.client.queryTime')">
          {{ (queryResult.duration / 1e6).toFixed(2) }}ms
        </TDescriptionsItem>
        <TDescriptionsItem :label="t('dns.client.server')">{{ queryResult.upstream }}</TDescriptionsItem>
        <TDescriptionsItem :label="t('dns.client.source')">{{ queryResult.source || t('dns.client.unknownSource') }}</TDescriptionsItem>
      </TDescriptions>
      <TTable :columns="answerColumns" :data="queryResult.answers || []" row-key="name" size="small" stripe hover />
    </TCard>

    <TDialog
      v-model:visible="showImport"
      :header="t('dns.client.importToZone')"
      :confirm-btn="{ loading: importing, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="520px"
      @confirm="handleImportSubmit"
    >
      <TForm ref="importFormRef" :data="importForm" :rules="importRules" label-align="top">
        <TFormItem :label="t('dns.zones.title')" name="zone_id">
          <TSelect v-model="importForm.zone_id" :options="zoneOptions" filterable :placeholder="t('dns.client.selectZone')" />
        </TFormItem>
        <TFormItem :label="t('dns.records.recordName')" name="name">
          <TInput v-model="importForm.name" />
        </TFormItem>
        <TFormItem :label="t('dns.records.recordType')">
          <TInput v-model="importForm.type" disabled />
        </TFormItem>
        <TFormItem :label="t('dns.records.recordValue')" name="value">
          <TTextarea v-model="importForm.value" :autosize="{ minRows: 2, maxRows: 4 }" />
        </TFormItem>
        <TFormItem :label="t('dns.zones.ttl')">
          <TInputNumber v-model="importForm.ttl" :min="0" />
        </TFormItem>
      </TForm>
    </TDialog>
  </div>
</template>

<style scoped>
.td-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.td-page-header :deep(.t-card__body) {
  padding: 16px 20px;
}
.result-alert {
  margin-bottom: 16px;
}
.result-meta {
  margin-bottom: 16px;
}
</style>