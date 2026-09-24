<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(defineProps<{
  show: boolean
  title?: string
  message: string
  confirmText?: string
  cancelText?: string
}>(), {
  title: '',
  confirmText: '',
  cancelText: ''
})

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()

const { t } = useI18n()
const showModal = ref(props.show)

const confirmLabel = computed(() => props.confirmText || t('common.confirm'))
const cancelLabel = computed(() => props.cancelText || t('common.cancel'))

watch(() => props.show, (val) => { showModal.value = val })
watch(showModal, (val) => {
  emit('update:show', val)
})

function handleVisibilityChange(visible: boolean) {
  if (!visible) emit('cancel')
}

function handleConfirm() {
  emit('confirm')
  showModal.value = false
}

function handleCancel() {
  showModal.value = false
  emit('cancel')
}
</script>

<template>
  <NModal
    v-model:show="showModal"
    preset="card"
    :title="title"
    type="warning"
    style="width: 420px;"
    @update:show="handleVisibilityChange"
  >
    <p>{{ message }}</p>
    <template #footer>
      <NSpace justify="end">
        <NButton @click="handleCancel">{{ cancelLabel }}</NButton>
        <NButton type="primary" @click="handleConfirm">{{ confirmLabel }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>
