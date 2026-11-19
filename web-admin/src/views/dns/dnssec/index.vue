<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Switch as TSwitch, Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import { usePermission } from '@/composables/usePermission';
import { formatAPITime } from '@/utils/datetime';
import {
  deleteDNSSECKey,
  disableDNSSEC,
  enableDNSSEC,
  generateDNSSECKey,
  getDNSSECStatus,
  getNSEC3Params,
  getZoneDSRecords,
  listDNSZones,
  promoteDNSSECStandbyKeys,
  rotateDNSSECKeys,
  setNSEC3Params,
  toggleDNSSECKey,
  type DNSSECKey,
  type DNSSECStatus,
  type DNSZone,
  type DSInfo,
  type NSEC3Params
} from '@/service/api/goddi/dns';

defineOptions({ name: 'DNSDNSSECPage' });

const { t } = useI18n();
const perm = usePermission();

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

const zones = ref<DNSZone[]>([]);
const selectedZone = ref<string>('');
const zoneOptions = computed(() =>
  zones.value
    .filter(z => z.type !== 'allowed' && z.type !== 'blocked')
    .map(z => ({ label: z.name, value: z.id }))
);

const status = ref<DNSSECStatus | null>(null);
const dnssecOn = computed(() => Boolean(status.value?.dnssec_enabled));
const dsRecords = ref<DSInfo[]>([]);
const loading = ref(false);
const dsLoaded = ref(false);
const togglingId = ref<string | null>(null);

const nsec3Form = reactive<{ iterations: number; salt: string; optout: boolean }>({
  iterations: 0,
  salt: '',
  optout: false
});
const nsec3Saving = ref(false);

const showGenerateKey = ref(false);
const generateKeyType = ref<'KSK' | 'ZSK'>('KSK');
const generateKeyAlgorithm = ref('ECDSAP256SHA256');
const algorithmOptions = ['ECDSAP256SHA256', 'ECDSAP384SHA384', 'ED25519', 'RSASHA256', 'RSASHA512'].map(a => ({
  label: a,
  value: a
}));

const keyColumns: PrimaryTableCol[] = [
  { colKey: 'key_tag', title: t('dns.dnssec.keyTag'), width: 100 },
  { colKey: 'id', title: t('dns.dnssec.keyId'), width: 160, ellipsis: true },
  { colKey: 'algorithm', title: t('dns.dnssec.algorithm'), width: 160 },
  {
    colKey: 'key_type',
    title: t('dns.dnssec.keyType'),
    width: 100,
    cell: (h, { row }) =>
      h(
        TTag,
        { size: 'small', theme: (row as DNSSECKey).key_type === 'KSK' ? 'warning' : 'primary', variant: 'light' },
        () => String((row as DNSSECKey).key_type)
      )
  },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 100,
    cell: (h, { row }) => {
      const r = row as DNSSECKey;
      return h(TSwitch, {
        size: 'small',
        value: r.enabled ?? true,
        disabled: !perm.canWrite('dns'),
        loading: togglingId.value === r.id,
        onChange: (v: unknown) => void handleToggleKey(r, Boolean(v))
      });
    }
  },
  { colKey: 'created_at', title: t('dns.dnssec.createdAt'), width: 190, cell: (_h, { row }) => formatAPITime((row as DNSSECKey).created_at) },
  {
    colKey: 'op',
    title: t('common.action'),
    width: 100,
    cell: (h, { row }) =>
      h(
        TLink,
        {
          theme: 'danger',
          disabled: !perm.canDelete('dns'),
          onClick: () => void handleDeleteKey(row as DNSSECKey)
        },
        () => t('common.delete')
      )
  }
];

const dsColumns: PrimaryTableCol[] = [
  { colKey: 'key_tag', title: t('dns.dnssec.keyTag'), width: 100 },
  { colKey: 'algorithm', title: t('dns.dnssec.algorithm'), width: 160 },
  { colKey: 'digest_type', title: t('dns.dnssec.digestType'), width: 120 },
  { colKey: 'digest', title: t('dns.dnssec.digest'), ellipsis: true }
];

async function loadZones() {
  try {
    const res = await listDNSZones({ page: 1, page_size: 500 });
    zones.value = res.data ?? [];
    if (!selectedZone.value && zoneOptions.value.length) {
      selectedZone.value = zoneOptions.value[0].value;
    }
  } catch (err) {
    msgError(err);
  }
}

async function loadAll() {
  if (!selectedZone.value) {
    status.value = null;
    dsRecords.value = [];
    return;
  }
  loading.value = true;
  try {
    const [s, ds, n3] = await Promise.all([
      getDNSSECStatus(selectedZone.value),
      getZoneDSRecords(selectedZone.value).catch(() => [] as DSInfo[]),
      getNSEC3Params(selectedZone.value).catch(() => null as NSEC3Params | null)
    ]);
    status.value = s;
    dsRecords.value = ds ?? [];
    dsLoaded.value = true;
    if (n3) {
      nsec3Form.iterations = n3.iterations;
      nsec3Form.salt = n3.salt;
      nsec3Form.optout = n3.optout;
    }
  } catch (err) {
    msgError(err);
  } finally {
    loading.value = false;
  }
}

async function handleAction(action: 'enable' | 'disable' | 'rotate' | 'promote') {
  if (!perm.canWrite('dns') || !selectedZone.value) return;
  try {
    const id = selectedZone.value;
    if (action === 'enable') await enableDNSSEC(id);
    else if (action === 'disable') await disableDNSSEC(id);
    else if (action === 'rotate') await rotateDNSSECKeys(id);
    else await promoteDNSSECStandbyKeys(id);
    void MessagePlugin.success(t('common.success'));
    await loadAll();
  } catch (err) {
    msgError(err);
  }
}

async function handleGenerateKey() {
  if (!perm.canWrite('dns') || !selectedZone.value) return;
  try {
    await generateDNSSECKey(selectedZone.value, generateKeyType.value, generateKeyAlgorithm.value);
    void MessagePlugin.success(t('common.success'));
    showGenerateKey.value = false;
    await loadAll();
  } catch (err) {
    msgError(err);
  }
}

async function handleToggleKey(key: DNSSECKey, enabled: boolean) {
  if (!perm.canWrite('dns') || !selectedZone.value) return;
  togglingId.value = key.id;
  try {
    await toggleDNSSECKey(selectedZone.value, key.id, enabled);
    key.enabled = enabled;
    void MessagePlugin.success(t('common.updateSuccess'));
  } catch (err) {
    msgError(err);
    await loadAll();
  } finally {
    togglingId.value = null;
  }
}

function handleDeleteKey(key: DNSSECKey) {
  if (!perm.canDelete('dns') || !selectedZone.value) return;
  const dialog = DialogPlugin.confirm({
    header: t('common.delete'),
    body: t('dns.dnssec.deleteKeyConfirm', { tag: key.key_tag ?? key.id }),
    confirmBtn: { theme: 'danger', content: t('common.delete') },
    onConfirm: async () => {
      try {
        await deleteDNSSECKey(selectedZone.value, key.id);
        void MessagePlugin.success(t('common.success'));
        dialog.destroy();
        await loadAll();
      } catch (err) {
        msgError(err);
        dialog.destroy();
      }
    }
  });
}

async function handleSaveNSEC3() {
  if (!perm.canWrite('dns') || !selectedZone.value) return;
  nsec3Saving.value = true;
  try {
    await setNSEC3Params(selectedZone.value, {
      iterations: nsec3Form.iterations,
      salt: nsec3Form.salt,
      optout: nsec3Form.optout
    });
    void MessagePlugin.success(t('common.success'));
  } catch (err) {
    msgError(err);
  } finally {
    nsec3Saving.value = false;
  }
}

watch(selectedZone, () => void loadAll());
onMounted(async () => {
  await loadZones();
  await loadAll();
});
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <div class="td-page-title">
        <h2>{{ t('dns.dnssec.title') }}</h2>
      </div>
    </TCard>

    <TAlert theme="warning" :message="t('dns.dnssec.experimentalHint')" />

    <TCard :bordered="false">
      <TForm layout="inline" label-align="top">
        <TFormItem :label="t('dns.dnssec.zone')">
          <TSelect
            v-model="selectedZone"
            :options="zoneOptions"
            :loading="loading"
            :placeholder="t('dns.dnssec.selectZone')"
            filterable
            style="width: 320px"
          />
        </TFormItem>
        <TFormItem :label="t('dns.dnssec.status')">
          <TTag :theme="dnssecOn ? 'success' : 'default'" variant="light" size="large">
            {{ dnssecOn ? t('common.enabled') : t('common.disabled') }}
          </TTag>
        </TFormItem>
        <TFormItem :label="t('dns.dnssec.keyCount')">
          <TTag theme="primary" variant="light" size="large">{{ status?.keys?.length ?? 0 }}</TTag>
        </TFormItem>
      </TForm>
    </TCard>

    <TCard :bordered="false" :title="t('dns.dnssec.keyLifecycle')">
      <TSpace size="small" wrap class="section-actions">
        <TButton
          v-if="!dnssecOn && perm.canWrite('dns')"
          theme="warning"
          size="small"
          :disabled="!selectedZone"
          @click="handleAction('enable')"
        >
          {{ t('dns.dnssec.enable') }}
        </TButton>
        <TButton
          v-if="dnssecOn && perm.canWrite('dns')"
          theme="warning"
          size="small"
          @click="handleAction('disable')"
        >
          {{ t('dns.dnssec.disable') }}
        </TButton>
        <TButton v-if="dnssecOn && perm.canWrite('dns')" size="small" variant="outline" @click="handleAction('rotate')">
          {{ t('dns.dnssec.rotate') }}
        </TButton>
        <TButton v-if="perm.canWrite('dns')" size="small" variant="outline" :disabled="!selectedZone" @click="showGenerateKey = true">
          {{ t('dns.dnssec.generateKey') }}
        </TButton>
        <TButton v-if="perm.canWrite('dns')" size="small" variant="outline" @click="handleAction('promote')">
          {{ t('dns.dnssec.promote') }}
        </TButton>
        <TButton size="small" variant="outline" :disabled="!selectedZone" @click="loadAll">
          {{ t('common.refresh') }}
        </TButton>
      </TSpace>

      <TTable :columns="keyColumns" :data="status?.keys ?? []" row-key="id" :loading="loading" stripe hover />
      <TEmpty v-if="!loading && !(status?.keys?.length ?? 0)" :description="t('dns.dnssec.noKeys')" />
    </TCard>

    <TCard :bordered="false" :title="t('dns.dnssec.dsTitle')">
      <TTable :columns="dsColumns" :data="dsRecords" row-key="key_tag" stripe hover />
      <TEmpty v-if="dsLoaded && !dsRecords.length" :description="t('dns.dnssec.noDs')" />
      <p class="hint">{{ t('dns.dnssec.dsHint') }}</p>
    </TCard>

    <TCard :bordered="false" :title="t('dns.dnssec.nsec3Title')">
      <TForm layout="inline" label-align="top">
        <TFormItem :label="t('dns.dnssec.iterations')">
          <TInputNumber v-model="nsec3Form.iterations" :min="0" :max="100" :disabled="!perm.canWrite('dns')" />
        </TFormItem>
        <TFormItem :label="t('dns.dnssec.salt')">
          <TInput v-model="nsec3Form.salt" :placeholder="t('common.hexPlaceholder')" :disabled="!perm.canWrite('dns')" style="width: 240px" />
        </TFormItem>
        <TFormItem :label="t('dns.dnssec.optout')">
          <TSwitch v-model="nsec3Form.optout" :disabled="!perm.canWrite('dns')" />
        </TFormItem>
        <TFormItem>
          <TButton
            v-if="perm.canWrite('dns')"
            theme="primary"
            size="small"
            :loading="nsec3Saving"
            :disabled="!selectedZone"
            @click="handleSaveNSEC3"
          >
            {{ t('common.save') }}
          </TButton>
        </TFormItem>
      </TForm>
      <p class="hint">{{ t('dns.dnssec.nsec3Hint') }}</p>
    </TCard>

    <TDialog
      v-model:visible="showGenerateKey"
      :header="t('dns.dnssec.generateKey')"
      :confirm-btn="t('common.confirm')"
      :cancel-btn="t('common.cancel')"
      width="460px"
      @confirm="handleGenerateKey"
    >
      <TForm label-align="top">
        <TFormItem :label="t('dns.dnssec.keyType')">
          <TSelect
            v-model="generateKeyType"
            :options="[{ label: 'KSK', value: 'KSK' }, { label: 'ZSK', value: 'ZSK' }]"
          />
        </TFormItem>
        <TFormItem :label="t('dns.dnssec.algorithm')">
          <TSelect v-model="generateKeyAlgorithm" :options="algorithmOptions" />
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
.section-actions {
  margin-bottom: 12px;
}
.hint {
  margin: 12px 0 0;
  color: var(--td-text-color-secondary);
  font-size: 12px;
}
</style>