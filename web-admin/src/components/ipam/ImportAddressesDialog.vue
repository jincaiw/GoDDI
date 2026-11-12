<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin, Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import { usePermission } from '@/composables/usePermission';
import SectionTitle from '@/components/ipam/SectionTitle.vue';
import ReportSummary from '@/components/ipam/ImportReportSummary.vue';
import { ApiError } from '@/service/api/goddi/client';
import { importIPAMData, previewIPAMImport, type ImportPlan, type ImportReport } from '@/service/api/goddi/ipam';

const props = defineProps<{
  show: boolean;
  subnetId: string;
  subnetLabel: string;
}>();

const emit = defineEmits<{
  (event: 'update:show', value: boolean): void;
  (event: 'imported'): void;
}>();

const { t } = useI18n();
const perm = usePermission();
const visible = computed({
  get: () => props.show,
  set: (value: boolean) => emit('update:show', value)
});

const step = ref<'pick' | 'preview' | 'done'>('pick');
const csvText = ref('');
const fileName = ref('');
const previewing = ref(false);
const importing = ref(false);
const report = ref<ImportReport | null>(null);
/** True when the server refused the batch and returned the report with the 400. */
const rejected = ref(false);
const fileInput = ref<HTMLInputElement | null>(null);

const canPreview = computed(() => csvText.value.trim().length > 0 && perm.canRead('ipam'));
/** A file the preview rejects is a file the import rejects: same planner. */
const hasBlockingErrors = computed(() => (report.value?.errors.length ?? 0) > 0);

function close() {
  visible.value = false;
}

function reset() {
  step.value = 'pick';
  csvText.value = '';
  fileName.value = '';
  report.value = null;
  rejected.value = false;
}

function backToPick() {
  step.value = 'pick';
  report.value = null;
  rejected.value = false;
}

async function handleFileChange(e: Event) {
  const input = e.target as HTMLInputElement;
  const raw = input.files?.[0];
  if (!raw) return;
  fileName.value = raw.name;
  csvText.value = await raw.text();
  report.value = null;
  input.value = '';
}

function body() {
  return { type: 'addresses', format: 'csv', parent_id: props.subnetId, data: csvText.value };
}

/** A rejected batch carries its report on the error; without it the operator
 *  sees "import failed" and has to re-read the file by eye. */
function reportFromError(err: unknown): ImportReport | null {
  if (!(err instanceof ApiError)) return null;
  const payload = err.payload as ImportReport | undefined;
  if (!payload || !Array.isArray(payload.errors)) return null;
  return payload;
}

async function handlePreview() {
  if (!perm.canRead('ipam')) return;
  previewing.value = true;
  rejected.value = false;
  try {
    report.value = await previewIPAMImport(body());
    step.value = 'preview';
  } catch (err: unknown) {
    const fromError = reportFromError(err);
    if (fromError) {
      report.value = fromError;
      rejected.value = true;
      step.value = 'preview';
    } else {
      void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
    }
  } finally {
    previewing.value = false;
  }
}

async function handleImport() {
  if (!perm.canWrite('ipam')) return;
  importing.value = true;
  try {
    report.value = await importIPAMData(body());
    step.value = 'done';
    rejected.value = false;
    void MessagePlugin.success(t('ipam.import.appliedTitle'));
    emit('imported');
  } catch (err: unknown) {
    const fromError = reportFromError(err);
    if (fromError) {
      report.value = fromError;
      rejected.value = true;
      step.value = 'preview';
    } else {
      void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
    }
  } finally {
    importing.value = false;
  }
}

function actionLabel(action: string) {
  const map: Record<string, string> = {
    create: t('ipam.import.actionCreate'),
    update: t('ipam.import.actionUpdate'),
    unchanged: t('ipam.import.actionUnchanged')
  };
  return map[action] || action;
}

function actionTheme(action: string): 'success' | 'primary' | 'default' {
  const map: Record<string, 'success' | 'primary' | 'default'> = {
    create: 'success',
    update: 'primary',
    unchanged: 'default'
  };
  return map[action] || 'default';
}

const changeColumns: PrimaryTableCol[] = [
  { colKey: 'line', title: t('ipam.import.line'), width: 70 },
  { colKey: 'ip', title: t('ipam.import.ip'), width: 160 },
  {
    colKey: 'action',
    title: t('ipam.import.action'),
    width: 110,
    cell: (h, { row }) =>
      h(TTag, { size: 'small', theme: actionTheme((row as ImportPlan).action), variant: 'light' }, () =>
        actionLabel((row as ImportPlan).action)
      )
  },
  {
    colKey: 'status',
    title: t('ipam.import.statusChange'),
    width: 180,
    cell: (_h, { row }) => {
      const r = row as ImportPlan;
      return r.previous_status ? `${r.previous_status} → ${r.status}` : r.status;
    }
  },
  {
    colKey: 'changed_fields',
    title: t('ipam.import.changedFields'),
    cell: (_h, { row }) => {
      const r = row as ImportPlan;
      return r.changed_fields?.length ? r.changed_fields.join(', ') : '—';
    }
  }
];

watch(
  () => props.show,
  show => {
    if (show) reset();
  }
);
</script>

<template>
  <TDialog
    v-model:visible="visible"
    :header="t('ipam.import.title')"
    width="760px"
    :footer="false"
  >
    <TAlert v-if="subnetLabel" theme="info" :message="`${t('ipam.subnets.title')}: ${subnetLabel}`" class="mb-4" />

    <!-- Step 1: pick. The backend takes the file's text as a JSON field. -->
    <template v-if="step === 'pick'">
      <TButton variant="outline" @click="fileInput?.click()">{{ t('ipam.import.chooseFile') }}</TButton>
      <input ref="fileInput" type="file" accept=".csv,.txt" class="hidden-file" @change="handleFileChange" />
      <p v-if="fileName" class="hint">{{ fileName }}</p>
      <TTextarea
        v-model="csvText"
        class="mt-3"
        :autosize="{ minRows: 10, maxRows: 10 }"
        :placeholder="t('ipam.import.pastePlaceholder')"
      />
    </template>

    <!-- Steps 2/3: preview and import answer with the same shape on purpose. -->
    <template v-else>
      <TAlert
        :theme="step === 'preview' ? 'info' : 'success'"
        :message="step === 'preview' ? t('ipam.import.previewTitle') : t('ipam.import.appliedTitle')"
        class="mb-4"
      />
      <TAlert v-if="rejected" theme="error" :title="t('ipam.import.rejected')" :message="t('ipam.import.rejectedHint')" class="mb-4" />

      <ReportSummary v-if="report" :report="report" />

      <template v-if="report && report.errors.length > 0">
        <SectionTitle :text="t('ipam.import.errors')" :count="report.errors.length" />
        <TAlert theme="error">
          <ul class="error-list">
            <li v-for="(line, index) in report.errors" :key="index">{{ line }}</li>
          </ul>
        </TAlert>
      </template>

      <template v-if="report && report.changes.length > 0">
        <SectionTitle :text="t('ipam.import.changes')" :count="report.changes.length" />
        <TTable
          size="small"
          :columns="changeColumns"
          :data="report.changes"
          row-key="line"
          :max-height="240"
          stripe
          hover
        />
        <p v-if="report.truncated" class="hint">{{ t('ipam.import.truncated') }}</p>
      </template>
    </template>

    <div class="dialog-actions">
      <TButton v-if="step === 'pick'" variant="outline" @click="close">{{ t('common.cancel') }}</TButton>
      <TButton v-else-if="step === 'preview'" variant="outline" :disabled="importing" @click="backToPick">
        {{ t('ipam.import.back') }}
      </TButton>
      <TButton v-else variant="outline" @click="close">{{ t('ipam.import.close') }}</TButton>

      <TButton v-if="step === 'pick'" theme="primary" :loading="previewing" :disabled="!canPreview" @click="handlePreview">
        {{ t('ipam.import.preview') }}
      </TButton>
      <TButton
        v-else-if="step === 'preview'"
        theme="primary"
        :loading="importing"
        :disabled="hasBlockingErrors || !perm.canWrite('ipam')"
        @click="handleImport"
      >
        {{ t('ipam.import.confirm') }}
      </TButton>
    </div>
  </TDialog>
</template>

<style scoped>
.mb-4 {
  margin-bottom: 14px;
}
.mt-3 {
  margin-top: 12px;
}
.hidden-file {
  display: none;
}
.hint {
  margin: 8px 0 0;
  color: #999;
  font-size: 12px;
}
.error-list {
  margin: 0;
  padding-left: 18px;
}
.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 20px;
}
</style>
