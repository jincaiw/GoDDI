<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Switch as TSwitch, Tag as TTag } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  addAllowRule,
  deleteAllowRule,
  exportAllowRules,
  flushAllowRules,
  importAllowRules,
  listAllowRules,
  type AllowRule
} from '@/service/api/goddi/dns';

defineOptions({ name: 'DNSAllowedPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showAdd = ref(false);
const submitting = ref(false);
const importInput = ref<HTMLInputElement | null>(null);
const form = reactive({ pattern: '', match_type: 'exact', enabled: true });

const matchOptions = [
  { label: 'Exact', value: 'exact' },
  { label: 'Suffix', value: 'suffix' },
  { label: 'Wildcard', value: 'wildcard' },
  { label: 'Regex', value: 'regex' }
];

const formRules = {
  pattern: [{ required: true, message: t('dns.security.pattern') + ' required', type: 'error' }]
};

const columns: PrimaryTableCol[] = [
  { colKey: 'pattern', title: t('dns.security.pattern'), ellipsis: true },
  {
    colKey: 'match_type',
    title: t('dns.security.matchType'),
    width: 130,
    cell: (h, { row }) => h(TTag, { size: 'small', variant: 'light' }, () => String((row as AllowRule).match_type))
  },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 90,
    cell: (h, { row }) => h(TSwitch, { size: 'small', disabled: true, value: Boolean((row as AllowRule).enabled) })
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 100,
    cell: (h, { row }) =>
      h(
        TLink,
        { theme: 'danger', disabled: !perm.canDelete('dns'), onClick: () => confirmDelete((row as AllowRule).id) },
        () => t('common.delete')
      )
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

function openAdd() {
  if (!perm.canWrite('dns')) return;
  Object.assign(form, { pattern: '', match_type: 'exact', enabled: true });
  showAdd.value = true;
}

async function handleAdd() {
  if (!perm.canWrite('dns')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    await addAllowRule(form);
    void MessagePlugin.success(t('common.createSuccess'));
    showAdd.value = false;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    submitting.value = false;
  }
}

function confirmDelete(id: string) {
  if (!perm.canDelete('dns')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: t('common.deleteConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteAllowRule(id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

function confirmFlush() {
  if (!perm.canDelete('dns')) return;
  const dlg = DialogPlugin.confirm({
    header: t('dns.security.flush'),
    body: t('dns.security.flushConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        const res = await flushAllowRules();
        void MessagePlugin.success(`${t('dns.security.flushSuccess')} (${res.flushed})`);
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

function triggerImport() {
  if (!perm.canWrite('dns')) return;
  importInput.value?.click();
}

async function handleImportFile(e: Event) {
  if (!perm.canWrite('dns')) return;
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  try {
    const text = await file.text();
    const response = await importAllowRules(text);
    // postRaw returns an AxiosResponse; the payload is the envelope's `data`.
    const result = (response.data as { data?: { imported: number; skipped: number } })?.data;
    void MessagePlugin.success(
      `${t('dns.security.importSuccess')} (${result?.imported ?? 0}, skipped ${result?.skipped ?? 0})`
    );
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    input.value = '';
  }
}

async function handleExport() {
  try {
    const response = await exportAllowRules();
    const url = window.URL.createObjectURL(new Blob([response.data as BlobPart]));
    const link = document.createElement('a');
    link.href = url;
    link.download = 'allowlist.txt';
    link.click();
    window.URL.revokeObjectURL(url);
  } catch (err) {
    msgError(err);
  }
}
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('dns.security.allowedTitle') }}</h2>
      <TSpace size="small">
        <TButton v-if="perm.canRead('dns')" variant="outline" @click="handleExport">
          {{ t('dns.security.exportRules') }}
        </TButton>
        <TButton v-if="perm.canWrite('dns')" variant="outline" @click="triggerImport">
          {{ t('dns.security.importRules') }}
        </TButton>
        <TButton v-if="perm.canDelete('dns')" theme="danger" variant="outline" @click="confirmFlush">
          {{ t('dns.security.flush') }}
        </TButton>
        <TButton v-if="perm.canWrite('dns')" theme="primary" @click="openAdd">
          <template #icon><TIcon name="add" /></template>
          {{ t('dns.security.addAllowRule') }}
        </TButton>
      </TSpace>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listAllowRules" :show-pagination="false" />

    <input ref="importInput" type="file" accept=".txt,.conf,text/plain" class="hidden-file" @change="handleImportFile" />

    <TDialog
      v-model:visible="showAdd"
      :header="t('dns.security.addAllowRule')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="480px"
      @confirm="handleAdd"
    >
      <TForm ref="formRef" :data="form" :rules="formRules" label-align="top">
        <TFormItem :label="t('dns.security.pattern')" name="pattern">
          <TInput v-model="form.pattern" />
        </TFormItem>
        <TFormItem :label="t('dns.security.matchType')" name="match_type">
          <TSelect v-model="form.match_type" :options="matchOptions" />
        </TFormItem>
        <TFormItem :label="t('common.enabled')" name="enabled">
          <TSwitch v-model="form.enabled" />
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
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 20px;
}
.td-page-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  white-space: nowrap;
}
.hidden-file {
  display: none;
}
</style>
