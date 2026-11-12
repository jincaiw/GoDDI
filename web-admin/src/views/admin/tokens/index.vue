<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  createAPIToken,
  deleteAPIToken,
  listAPITokens,
  type APIToken
} from '@/service/api/goddi/user';

defineOptions({ name: 'AdminTokensPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showCreate = ref(false);
const showToken = ref(false);
const creating = ref(false);
const createdToken = ref('');

const createForm = reactive<{ name: string; description: string; expires_at: number | null }>({
  name: '',
  description: '',
  expires_at: null
});

const formRules = {
  name: [{ required: true, message: t('admin.tokens.tokenName') + ' required', type: 'error' }]
};

const columns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('admin.tokens.tokenName') },
  { colKey: 'description', title: t('common.description'), ellipsis: true },
  { colKey: 'username', title: t('admin.users.username'), width: 140 },
  { colKey: 'expires_at', title: t('admin.tokens.expiresAt'), width: 170 },
  { colKey: 'last_used_at', title: t('admin.tokens.lastUsedAt'), width: 170 },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 100,
    cell: (h, { row }) =>
      h(
        TLink,
        { theme: 'danger', disabled: !perm.canDelete('token'), onClick: () => confirmDelete((row as APIToken).id) },
        () => t('common.delete')
      )
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

function openCreate() {
  if (!perm.canWrite('token')) return;
  Object.assign(createForm, { name: '', description: '', expires_at: null });
  showCreate.value = true;
}

async function handleCreate() {
  if (!perm.canWrite('token')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  creating.value = true;
  try {
    const result = await createAPIToken({
      name: createForm.name,
      description: createForm.description || undefined,
      expires_at: createForm.expires_at ? new Date(createForm.expires_at).toISOString() : undefined
    });
    createdToken.value = result.token;
    showCreate.value = false;
    showToken.value = true;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    creating.value = false;
  }
}

function confirmDelete(id: string) {
  if (!perm.canDelete('token')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: t('common.deleteConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteAPIToken(id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

function copyToken() {
  void navigator.clipboard.writeText(createdToken.value).then(
    () => void MessagePlugin.success('Copied!'),
    () => void MessagePlugin.warning('Clipboard unavailable')
  );
}
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('admin.tokens.title') }}</h2>
      <TButton v-if="perm.canWrite('token')" theme="primary" @click="openCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('admin.tokens.createToken') }}
      </TButton>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listAPITokens" />

    <TDialog
      v-model:visible="showCreate"
      :header="t('admin.tokens.createToken')"
      :confirm-btn="{ loading: creating, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="480px"
      @confirm="handleCreate"
    >
      <TForm ref="formRef" :data="createForm" :rules="formRules" label-align="top">
        <TFormItem :label="t('admin.tokens.tokenName')" name="name">
          <TInput v-model="createForm.name" :disabled="!perm.canWrite('token')" />
        </TFormItem>
        <TFormItem :label="t('common.descriptions')" name="description">
          <TTextarea v-model="createForm.description" :disabled="!perm.canWrite('token')" :autosize="{ minRows: 2 }" />
        </TFormItem>
        <TFormItem :label="t('admin.tokens.expiresAt')" name="expires_at">
          <TDatePicker
            v-model="createForm.expires_at"
            enable-time-picker
            clearable
            style="width: 100%"
            :disabled="!perm.canWrite('token')"
          />
        </TFormItem>
      </TForm>
    </TDialog>

    <TDialog
      v-model:visible="showToken"
      :header="t('admin.tokens.tokenCreated')"
      :footer="false"
      :close-on-overlay-click="false"
      width="520px"
    >
      <TAlert theme="warning" :message="t('admin.tokens.tokenWarning')" class="token-alert" />
      <TTextarea :value="createdToken" readonly :autosize="{ minRows: 3 }" />
      <div class="token-actions">
        <TButton theme="primary" @click="copyToken">{{ t('admin.tokens.copyToken') }}</TButton>
        <TButton variant="outline" @click="showToken = false">{{ t('common.confirm') }}</TButton>
      </div>
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
.token-alert {
  margin-bottom: 16px;
}
.token-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 16px;
}
</style>
