<script setup lang="ts">
import { namedDeleteBody } from '@/utils/confirm';
import { computed, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Tag as TTag } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import AddressDetailDrawer from '@/components/ipam/AddressDetailDrawer.vue';
import { usePermission } from '@/composables/usePermission';
import { ipamStatusLabel, ipamStatusTheme } from '@/composables/useIpamLifecycle';
import {
  allocateIP,
  listIPAMAddresses,
  listIPAMSubnets,
  releaseIP,
  type AllocateIPRequest,
  type IPAMAddress
} from '@/service/api/goddi/ipam';

defineOptions({ name: 'IPAMAddressesPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showAllocate = ref(false);
const allocating = ref(false);
const searchQuery = ref('');
const statusFilter = ref<string | undefined>(undefined);
const subnetOptions = ref<Array<{ label: string; value: string }>>([]);

// The 360° view is keyed by (space, ip), not by row id: the same address in
// another space is a different record.
const showDetail = ref(false);
const detailSpaceId = ref('');
const detailIp = ref('');

const allocateForm = reactive<AllocateIPRequest>({
  subnet_id: '',
  ip_address: '',
  hostname: '',
  mac_address: '',
  owner: '',
  device: '',
  location: '',
  description: ''
});

const formRules = {
  subnet_id: [{ required: true, message: t('ipam.subnets.title') + t('common.required'), type: 'error' }]
};

const statusOptions = [
  { label: t('ipam.addresses.statusUsed'), value: 'used' },
  { label: t('ipam.addresses.statusFree'), value: 'available' },
  { label: t('ipam.addresses.statusReserved'), value: 'reserved' },
  { label: t('ipam.addresses.statusConflict'), value: 'conflict' }
];


const params = computed<Record<string, unknown>>(() => {
  const p: Record<string, unknown> = {};
  if (searchQuery.value) p.search = searchQuery.value;
  if (statusFilter.value) p.status = statusFilter.value;
  return p;
});

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

const columns: PrimaryTableCol[] = [
  { colKey: 'ip_address', title: t('ipam.addresses.ip'), width: 150 },
  {
    colKey: 'status',
    title: t('common.status'),
    width: 110,
    cell: (h, { row }) => {
      const s = String((row as IPAMAddress).status);
      return h(TTag, { size: 'small', theme: ipamStatusTheme(s), variant: 'light' }, () => ipamStatusLabel(t, s));
    }
  },
  { colKey: 'hostname', title: t('ipam.addresses.hostname'), ellipsis: true },
  { colKey: 'mac_address', title: t('ipam.addresses.mac'), ellipsis: true },
  { colKey: 'description', title: t('common.description'), ellipsis: true },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 190,
    cell: (h, { row }) => {
      const r = row as IPAMAddress;
      return h('div', { class: 'row-actions' }, [
        h(TLink, { theme: 'primary', onClick: () => openDetail(r) }, () => t('ipam.addresses.viewDetail')),
        ...(r.status === 'used'
          ? [h(TLink, { theme: 'warning', disabled: !perm.canWrite('ipam'), onClick: () => confirmRelease(r.id) }, () => t('ipam.addresses.release'))]
          : [])
      ]);
    }
  }
];

function openDetail(row: IPAMAddress) {
  detailSpaceId.value = row.space_id;
  detailIp.value = row.ip_address;
  showDetail.value = true;
}

async function loadSubnets() {
  try {
    const result = await listIPAMSubnets({ page: 1, page_size: 100 });
    subnetOptions.value = result.data.map(s => ({ label: `${s.name} (${s.cidr})`, value: s.id }));
  } catch {
    // Subnet picker is optional for the list; it will simply be empty.
  }
}

function openAllocate() {
  if (!perm.canWrite('ipam')) return;
  Object.assign(allocateForm, {
    subnet_id: '',
    ip_address: '',
    hostname: '',
    mac_address: '',
    owner: '',
    device: '',
    location: '',
    description: ''
  });
  showAllocate.value = true;
}

async function handleAllocate() {
  if (!perm.canWrite('ipam')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  allocating.value = true;
  try {
    await allocateIP(allocateForm);
    void MessagePlugin.success(t('common.createSuccess'));
    showAllocate.value = false;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    allocating.value = false;
  }
}

function confirmRelease(id: string) {
  if (!perm.canWrite('ipam')) return;
  const dlg = DialogPlugin.confirm({
    header: t('ipam.addresses.release'),
    body: namedDeleteBody(id),
    theme: 'warning',
    onConfirm: async () => {
      try {
        await releaseIP(id);
        void MessagePlugin.success(t('common.success'));
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

void loadSubnets();
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('ipam.addresses.title') }}</h2>
      <TSpace size="small">
        <TButton v-if="perm.canWrite('ipam')" theme="primary" @click="openAllocate">
          <template #icon><TIcon name="add" /></template>
          {{ t('ipam.addresses.allocate') }}
        </TButton>
        <TButton variant="outline" @click="tableRef?.reload()">{{ t('common.refresh') }}</TButton>
      </TSpace>
    </TCard>

    <TCard :bordered="false" class="filter-bar">
      <TSpace size="small">
        <TInput
          v-model="searchQuery"
          clearable
          :placeholder="t('common.search')"
          style="width: 240px"
          @enter="tableRef?.reload()"
        />
        <TSelect v-model="statusFilter" :options="statusOptions" clearable :placeholder="t('common.status')" style="width: 160px" />
      </TSpace>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listIPAMAddresses" :params="params" />

    <TDialog
      v-model:visible="showAllocate"
      :header="t('ipam.addresses.allocate')"
      :confirm-btn="{ loading: allocating, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="520px"
      @confirm="handleAllocate"
    >
      <TForm ref="formRef" :data="allocateForm" :rules="formRules" label-align="top">
        <TFormItem :label="t('ipam.subnets.title')" name="subnet_id">
          <TSelect v-model="allocateForm.subnet_id" :options="subnetOptions" />
        </TFormItem>
        <TFormItem :label="t('ipam.addresses.ip')">
          <TInput v-model="allocateForm.ip_address" :placeholder="t('common.autoAssign')" />
        </TFormItem>
        <TFormItem :label="t('ipam.addresses.hostname')">
          <TInput v-model="allocateForm.hostname" />
        </TFormItem>
        <TFormItem :label="t('ipam.addresses.mac')">
          <TInput v-model="allocateForm.mac_address" />
        </TFormItem>
        <TFormItem :label="t('ipam.addresses.owner')">
          <TInput v-model="allocateForm.owner" />
        </TFormItem>
        <TFormItem :label="t('ipam.addresses.device')">
          <TInput v-model="allocateForm.device" />
        </TFormItem>
        <TFormItem :label="t('ipam.addresses.location')">
          <TInput v-model="allocateForm.location" />
        </TFormItem>
        <TFormItem :label="t('common.descriptions')">
          <TInput v-model="allocateForm.description" />
        </TFormItem>
      </TForm>
    </TDialog>

    <AddressDetailDrawer v-model:show="showDetail" :space-id="detailSpaceId" :ip="detailIp" />
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
.filter-bar :deep(.t-card__body) {
  padding: 12px 16px;
}
:deep(.row-actions) {
  display: flex;
  gap: 12px;
}
</style>