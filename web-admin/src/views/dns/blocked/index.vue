<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Space as TSpace, Switch as TSwitch, Tag as TTag } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  addBlockRule,
  createBlockList,
  deleteBlockList,
  deleteBlockRule,
  flushBlockLists,
  getBlockingStatus,
  listBlockLists,
  listBlockRules,
  refreshBlockList,
  temporaryDisableBlocking,
  type BlockList,
  type BlockRule,
  type BlockingStatus
} from '@/service/api/goddi/dns';

defineOptions({ name: 'DNSBlockedPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const listFormRef = ref<FormInstanceFunctions | null>(null);
const ruleFormRef = ref<FormInstanceFunctions | null>(null);
const showList = ref(false);
const showRule = ref(false);
const listSubmitting = ref(false);
const ruleSubmitting = ref(false);
const refreshingId = ref<string | null>(null);

const selectedList = ref<BlockList | null>(null);
const blockRules = ref<BlockRule[]>([]);
const ruleLoading = ref(false);

const listForm = reactive({ name: '', type: 'custom', url: '', enabled: true });
const ruleForm = reactive({ pattern: '', match_type: 'suffix', response_type: 'NXDOMAIN', response_data: '', enabled: true });

const blockingStatus = ref<BlockingStatus | null>(null);
const disableMinutes = ref<number>(10);
const disableOptions = [5, 10, 15, 30, 60, 120, 240].map(m => ({ label: `${m} min`, value: m }));
let statusTimer: ReturnType<typeof setInterval> | null = null;

const matchOptions = [
  { label: 'Exact', value: 'exact' },
  { label: 'Suffix', value: 'suffix' },
  { label: 'Wildcard', value: 'wildcard' },
  { label: 'Regex', value: 'regex' }
];
const responseOptions = [
  { label: 'NXDOMAIN', value: 'NXDOMAIN' },
  { label: 'NODATA', value: 'NODATA' },
  { label: 'REFUSED', value: 'REFUSED' },
  { label: 'DROP', value: 'DROP' },
  { label: 'Custom IP', value: 'CUSTOM_IP' }
];

const listRules = {
  name: [{ required: true, message: t('common.name') + ' required', type: 'error' }]
};
const ruleRules = {
  pattern: [{ required: true, message: t('dns.security.pattern') + ' required', type: 'error' }]
};

function formatTime(value: string | null): string {
  if (!value) return '-';
  const d = new Date(value);
  return isNaN(d.getTime()) ? value : d.toLocaleString();
}

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

const listColumns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('common.name') },
  {
    colKey: 'type',
    title: t('common.type'),
    width: 110,
    cell: (h, { row }) => h(TTag, { size: 'small', variant: 'light' }, () => String((row as BlockList).type))
  },
  { colKey: 'entry_count', title: 'Rules', width: 90 },
  {
    colKey: 'last_fetch',
    title: t('dns.security.lastFetch'),
    width: 190,
    cell: (h, { row }) => {
      const r = row as BlockList;
      if (r.type !== 'external') return '-';
      if (!r.last_fetch_at) return h(TTag, { size: 'small' }, () => t('dns.security.neverFetched'));
      return h(TSpace, { size: 'small', align: 'center' }, () => [
        h(
          TTag,
          { size: 'small', theme: r.last_fetch_status === 'ok' ? 'success' : 'danger', variant: 'light' },
          () => (r.last_fetch_status === 'ok' ? 'OK' : t('dns.security.fetchFailed'))
        ),
        h('span', { class: 'fetch-time' }, formatTime(r.last_fetch_at))
      ]);
    }
  },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 90,
    cell: (h, { row }) => h(TSwitch, { size: 'small', disabled: true, value: Boolean((row as BlockList).enabled) })
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 220,
    cell: (h, { row }) => {
      const r = row as BlockList;
      return h(TSpace, { size: 'small' }, () => [
        h(TLink, { theme: 'primary', onClick: () => void selectList(r) }, () => 'Rules'),
        ...(r.type === 'external'
          ? [
              h(
                TLink,
                {
                  theme: 'primary',
                  disabled: !perm.canWrite('dns'),
                  loading: refreshingId.value === r.id,
                  onClick: () => void handleRefresh(r.id)
                },
                () => t('dns.security.refresh')
              )
            ]
          : []),
        h(
          TLink,
          { theme: 'danger', disabled: !perm.canDelete('dns'), onClick: () => confirmDeleteList(r.id) },
          () => t('common.delete')
        )
      ]);
    }
  }
];

const ruleColumns: PrimaryTableCol[] = [
  { colKey: 'pattern', title: t('dns.security.pattern'), ellipsis: true },
  {
    colKey: 'match_type',
    title: t('dns.security.matchType'),
    width: 120,
    cell: (h, { row }) => h(TTag, { size: 'small', variant: 'light' }, () => String((row as BlockRule).match_type))
  },
  {
    colKey: 'response_type',
    title: t('dns.security.responseType'),
    width: 130,
    cell: (h, { row }) =>
      h(
        TTag,
        { size: 'small', theme: (row as BlockRule).response_type === 'DROP' ? 'danger' : 'warning', variant: 'light' },
        () => String((row as BlockRule).response_type)
      )
  },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 90,
    cell: (h, { row }) => h(TSwitch, { size: 'small', disabled: true, value: Boolean((row as BlockRule).enabled) })
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 90,
    cell: (h, { row }) => {
      const r = row as BlockRule;
      return h(
        TLink,
        { theme: 'danger', disabled: !perm.canDelete('dns'), onClick: () => void handleDeleteRule(r.list_id, r.id) },
        () => t('common.delete')
      );
    }
  }
];

async function loadBlockingStatus() {
  try {
    blockingStatus.value = await getBlockingStatus();
  } catch {
    // Status banner is advisory; ignore polling failures.
  }
}

async function handleTemporaryDisable() {
  if (!perm.canWrite('dns')) return;
  try {
    blockingStatus.value = await temporaryDisableBlocking(disableMinutes.value);
    void MessagePlugin.success(t('dns.security.disableSuccess'));
  } catch (err) {
    msgError(err);
  }
}

async function handleResumeBlocking() {
  if (!perm.canWrite('dns')) return;
  try {
    await temporaryDisableBlocking(0);
    blockingStatus.value = await getBlockingStatus();
    void MessagePlugin.success(t('dns.security.resumeSuccess'));
  } catch (err) {
    msgError(err);
  }
}

async function handleRefresh(id: string) {
  if (!perm.canWrite('dns')) return;
  refreshingId.value = id;
  try {
    await refreshBlockList(id);
    void MessagePlugin.success(t('dns.security.refreshSuccess'));
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    refreshingId.value = null;
  }
}

async function selectList(row: BlockList) {
  selectedList.value = row;
  await loadBlockRules(row.id);
}

async function loadBlockRules(listId: string) {
  ruleLoading.value = true;
  try {
    blockRules.value = (await listBlockRules(listId)).data;
  } catch (err) {
    msgError(err);
  } finally {
    ruleLoading.value = false;
  }
}

function openCreateList() {
  if (!perm.canWrite('dns')) return;
  Object.assign(listForm, { name: '', type: 'custom', url: '', enabled: true });
  showList.value = true;
}

async function handleCreateList() {
  if (!perm.canWrite('dns')) return;
  const valid = await listFormRef.value?.validate();
  if (valid !== true) return;
  listSubmitting.value = true;
  try {
    await createBlockList(listForm);
    void MessagePlugin.success(t('common.createSuccess'));
    showList.value = false;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    listSubmitting.value = false;
  }
}

function confirmDeleteList(id: string) {
  if (!perm.canDelete('dns')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: t('common.deleteConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteBlockList(id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        selectedList.value = null;
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

function openRule() {
  if (!perm.canWrite('dns') || !selectedList.value) return;
  Object.assign(ruleForm, {
    pattern: '',
    match_type: 'suffix',
    response_type: 'NXDOMAIN',
    response_data: '',
    enabled: true
  });
  showRule.value = true;
}

async function handleAddRule() {
  if (!perm.canWrite('dns') || !selectedList.value) return;
  const valid = await ruleFormRef.value?.validate();
  if (valid !== true) return;
  ruleSubmitting.value = true;
  try {
    await addBlockRule(selectedList.value.id, ruleForm);
    void MessagePlugin.success(t('common.createSuccess'));
    showRule.value = false;
    await loadBlockRules(selectedList.value.id);
  } catch (err) {
    msgError(err);
  } finally {
    ruleSubmitting.value = false;
  }
}

async function handleDeleteRule(listId: string, ruleId: string) {
  if (!perm.canDelete('dns')) return;
  try {
    await deleteBlockRule(listId, ruleId);
    void MessagePlugin.success(t('common.deleteSuccess'));
    if (selectedList.value) await loadBlockRules(selectedList.value.id);
  } catch (err) {
    msgError(err);
  }
}

function confirmFlush() {
  if (!perm.canDelete('dns')) return;
  const dlg = DialogPlugin.confirm({
    header: t('dns.security.flush'),
    body: t('dns.security.flushConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        const res = await flushBlockLists();
        void MessagePlugin.success(`${t('dns.security.flushSuccess')} (${res.flushed})`);
        selectedList.value = null;
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

onMounted(() => {
  void loadBlockingStatus();
  statusTimer = setInterval(loadBlockingStatus, 30000);
});
onUnmounted(() => {
  if (statusTimer) clearInterval(statusTimer);
});
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('dns.security.blockedTitle') }}</h2>
      <TSpace size="small">
        <TButton v-if="perm.canDelete('dns')" theme="danger" variant="outline" @click="confirmFlush">
          {{ t('dns.security.flush') }}
        </TButton>
        <TButton v-if="perm.canWrite('dns')" theme="primary" @click="openCreateList">
          <template #icon><TIcon name="add" /></template>
          {{ t('dns.security.createBlockList') }}
        </TButton>
      </TSpace>
    </TCard>

    <TAlert
      v-if="blockingStatus && blockingStatus.disabled_until"
      theme="warning"
      :message="`${t('dns.security.blockingDisabledUntil')} ${formatTime(blockingStatus.disabled_until)}`"
      close
    >
      <template #operation>
        <TButton v-if="perm.canWrite('dns')" size="small" @click="handleResumeBlocking">
          {{ t('dns.security.resumeBlocking') }}
        </TButton>
      </template>
    </TAlert>
    <TCard v-else :bordered="false" class="disable-bar">
      <TSpace size="small" align="center">
        <span>{{ t('dns.security.temporaryDisable') }}</span>
        <TSelect v-model="disableMinutes" :options="disableOptions" style="width: 130px" />
        <TButton v-if="perm.canWrite('dns')" size="small" theme="warning" @click="handleTemporaryDisable">
          {{ t('dns.security.disableNow') }}
        </TButton>
      </TSpace>
    </TCard>

    <RemoteTable ref="tableRef" :columns="listColumns" :fetcher="listBlockLists" :show-pagination="false" />

    <TCard v-if="selectedList" :bordered="false" :title="`${selectedList.name} - Rules`">
      <template #actions>
        <TButton v-if="perm.canWrite('dns')" size="small" theme="primary" @click="openRule">
          {{ t('dns.security.addBlockRule') }}
        </TButton>
      </template>
      <TTable :columns="ruleColumns" :data="blockRules" :loading="ruleLoading" row-key="id" size="small" stripe hover />
    </TCard>

    <TDialog
      v-model:visible="showList"
      :header="t('dns.security.createBlockList')"
      :confirm-btn="{ loading: listSubmitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="480px"
      @confirm="handleCreateList"
    >
      <TForm ref="listFormRef" :data="listForm" :rules="listRules" label-align="top">
        <TFormItem :label="t('common.name')" name="name">
          <TInput v-model="listForm.name" />
        </TFormItem>
        <TFormItem :label="t('common.type')" name="type">
          <TSelect
            v-model="listForm.type"
            :options="[{ label: 'Custom', value: 'custom' }, { label: 'External', value: 'external' }]"
          />
        </TFormItem>
        <TFormItem v-if="listForm.type === 'external'" label="URL">
          <TInput v-model="listForm.url" placeholder="https://example.com/blocklist.txt" />
        </TFormItem>
        <TFormItem :label="t('common.enabled')">
          <TSwitch v-model="listForm.enabled" />
        </TFormItem>
      </TForm>
    </TDialog>

    <TDialog
      v-model:visible="showRule"
      :header="t('dns.security.addBlockRule')"
      :confirm-btn="{ loading: ruleSubmitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="500px"
      @confirm="handleAddRule"
    >
      <TForm ref="ruleFormRef" :data="ruleForm" :rules="ruleRules" label-align="top">
        <TFormItem :label="t('dns.security.pattern')" name="pattern">
          <TInput v-model="ruleForm.pattern" />
        </TFormItem>
        <TFormItem :label="t('dns.security.matchType')">
          <TSelect v-model="ruleForm.match_type" :options="matchOptions" />
        </TFormItem>
        <TFormItem :label="t('dns.security.responseType')">
          <TSelect v-model="ruleForm.response_type" :options="responseOptions" />
        </TFormItem>
        <TFormItem v-if="ruleForm.response_type === 'CUSTOM_IP'" label="IP">
          <TInput v-model="ruleForm.response_data" placeholder="0.0.0.0" />
        </TFormItem>
        <TFormItem :label="t('common.enabled')">
          <TSwitch v-model="ruleForm.enabled" />
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
  padding: 16px 20px;
}
.td-page-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
.disable-bar :deep(.t-card__body) {
  padding: 12px 16px;
}
:deep(.fetch-time) {
  font-size: 12px;
  color: #9ca3af;
}
</style>
