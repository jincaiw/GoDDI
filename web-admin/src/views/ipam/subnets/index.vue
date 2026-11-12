<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Space as TSpace } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import ImportAddressesDialog from '@/components/ipam/ImportAddressesDialog.vue';
import PoolWizardDialog from '@/components/ipam/PoolWizardDialog.vue';
import ReverseZoneDialog from '@/components/ipam/ReverseZoneDialog.vue';
import { usePermission } from '@/composables/usePermission';
import {
  createIPAMSubnet,
  deleteIPAMSubnet,
  listIPAMSpaces,
  listIPAMSubnets,
  updateIPAMSubnet,
  type CreateIPAMSubnetRequest,
  type IPAMSubnet
} from '@/service/api/goddi/ipam';

defineOptions({ name: 'IPAMSubnetsPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showModal = ref(false);
const submitting = ref(false);
const editing = ref<IPAMSubnet | null>(null);
const spaceOptions = ref<Array<{ label: string; value: string }>>([]);

// Each dialog targets one subnet: the row's outcome depends on which subnet
// the address lands in, so the entry point carries it rather than asking again.
const showImport = ref(false);
const importSubnetId = ref('');
const importSubnetLabel = ref('');
const showPool = ref(false);
const poolSubnetId = ref('');
const poolSubnetLabel = ref('');
const showReverseZone = ref(false);
const reverseZoneSubnetId = ref('');
const reverseZoneSubnetLabel = ref('');

const formData = reactive<CreateIPAMSubnetRequest>({
  name: '',
  space_id: '',
  cidr: '',
  vlan_id: undefined,
  location: '',
  description: ''
});

const formRules = {
  name: [{ required: true, message: t('common.name') + ' required', type: 'error' }],
  space_id: [{ required: true, message: t('ipam.spaces.title') + ' required', type: 'error' }],
  cidr: [{ required: true, message: t('ipam.subnets.cidr') + ' required', type: 'error' }]
};

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

function subnetLabel(row: IPAMSubnet) {
  return `${row.name} (${row.cidr})`;
}

const columns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('common.name') },
  { colKey: 'cidr', title: t('ipam.subnets.cidr') },
  { colKey: 'vlan_id', title: t('ipam.subnets.vlanId'), width: 110 },
  { colKey: 'location', title: t('ipam.subnets.location'), ellipsis: true },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 340,
    cell: (h, { row }) => {
      const r = row as IPAMSubnet;
      return h(TSpace, { size: 'small' }, () => [
        h(TLink, { theme: 'primary', disabled: !perm.canWrite('ipam'), onClick: () => openEdit(r) }, () => t('common.edit')),
        h(TLink, { theme: 'primary', disabled: !perm.canRead('ipam'), onClick: () => openImport(r) }, () => t('ipam.subnets.importAddresses')),
        h(TLink, { theme: 'primary', disabled: !perm.canWrite('ipam'), onClick: () => openPool(r) }, () => t('ipam.subnets.createDhcpScope')),
        h(TLink, { theme: 'primary', disabled: !perm.canWrite('ipam'), onClick: () => openReverseZone(r) }, () => t('ipam.subnets.generateReverseZone')),
        h(TLink, { theme: 'danger', disabled: !perm.canDelete('ipam'), onClick: () => confirmDelete(r.id) }, () => t('common.delete'))
      ]);
    }
  }
];

/** Wraps getList to also refresh the space picker in the same pass. */
async function fetchSubnets(params: Record<string, unknown>) {
  const [result, spaces] = await Promise.all([
    listIPAMSubnets(params),
    listIPAMSpaces({ page: 1, page_size: 100 })
  ]);
  spaceOptions.value = spaces.data.map(s => ({ label: s.name, value: s.id }));
  return result;
}

function openImport(row: IPAMSubnet) {
  if (!perm.canRead('ipam')) return;
  importSubnetId.value = row.id;
  importSubnetLabel.value = subnetLabel(row);
  showImport.value = true;
}

function openPool(row: IPAMSubnet) {
  if (!perm.canWrite('ipam')) return;
  poolSubnetId.value = row.id;
  poolSubnetLabel.value = subnetLabel(row);
  showPool.value = true;
}

function openReverseZone(row: IPAMSubnet) {
  if (!perm.canWrite('ipam')) return;
  reverseZoneSubnetId.value = row.id;
  reverseZoneSubnetLabel.value = subnetLabel(row);
  showReverseZone.value = true;
}

function openCreate() {
  if (!perm.canWrite('ipam')) return;
  editing.value = null;
  Object.assign(formData, {
    name: '',
    space_id: '',
    cidr: '',
    vlan_id: undefined,
    location: '',
    description: ''
  });
  showModal.value = true;
}

function openEdit(row: IPAMSubnet) {
  if (!perm.canWrite('ipam')) return;
  editing.value = row;
  Object.assign(formData, {
    name: row.name,
    space_id: row.space_id,
    cidr: row.cidr,
    vlan_id: row.vlan_id,
    location: row.location,
    description: row.description
  });
  showModal.value = true;
}

async function handleSubmit() {
  if (!perm.canWrite('ipam')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    if (editing.value) {
      await updateIPAMSubnet(editing.value.id, formData);
      void MessagePlugin.success(t('common.updateSuccess'));
    } else {
      await createIPAMSubnet(formData);
      void MessagePlugin.success(t('common.createSuccess'));
    }
    showModal.value = false;
    editing.value = null;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    submitting.value = false;
  }
}

function confirmDelete(id: string) {
  if (!perm.canDelete('ipam')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: t('common.deleteConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteIPAMSubnet(id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('ipam.subnets.title') }}</h2>
      <TSpace size="small">
        <TButton v-if="perm.canWrite('ipam')" theme="primary" @click="openCreate">
          <template #icon><TIcon name="add" /></template>
          {{ t('ipam.subnets.createSubnet') }}
        </TButton>
        <TButton variant="outline" @click="tableRef?.reload()">{{ t('common.refresh') }}</TButton>
      </TSpace>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="fetchSubnets" />

    <TDialog
      v-model:visible="showModal"
      :header="editing ? t('ipam.subnets.editSubnet') : t('ipam.subnets.createSubnet')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="560px"
      @confirm="handleSubmit"
    >
      <TForm ref="formRef" :data="formData" :rules="formRules" label-align="top">
        <TFormItem :label="t('common.name')" name="name">
          <TInput v-model="formData.name" />
        </TFormItem>
        <TFormItem :label="t('ipam.spaces.title')" name="space_id">
          <TSelect v-model="formData.space_id" :options="spaceOptions" />
        </TFormItem>
        <TFormItem :label="t('ipam.subnets.cidr')" name="cidr">
          <TInput v-model="formData.cidr" placeholder="192.168.1.0/24" />
        </TFormItem>
        <TFormItem :label="t('ipam.subnets.vlanId')">
          <TInputNumber v-model="formData.vlan_id" :min="1" :max="4094" clearable />
        </TFormItem>
        <TFormItem :label="t('ipam.subnets.location')">
          <TInput v-model="formData.location" />
        </TFormItem>
        <TFormItem :label="t('common.descriptions')">
          <TTextarea v-model="formData.description" :autosize="{ minRows: 2, maxRows: 4 }" />
        </TFormItem>
      </TForm>
    </TDialog>

    <ImportAddressesDialog
      v-model:show="showImport"
      :subnet-id="importSubnetId"
      :subnet-label="importSubnetLabel"
      @imported="tableRef?.reload()"
    />
    <PoolWizardDialog
      v-model:show="showPool"
      :subnet-id="poolSubnetId"
      :subnet-label="poolSubnetLabel"
      @created="tableRef?.reload()"
    />
    <ReverseZoneDialog
      v-model:show="showReverseZone"
      :subnet-id="reverseZoneSubnetId"
      :subnet-label="reverseZoneSubnetLabel"
      @created="tableRef?.reload()"
    />
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
</style>
