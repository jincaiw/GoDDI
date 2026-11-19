<script setup lang="ts">
import { TranslateIcon } from 'tdesign-icons-vue-next';
import { computed } from 'vue';
import { useAppStore } from '@/store/modules/app';
import { $t } from '@/locales';

defineOptions({ name: 'StarterLanguageSwitcher' });

const appStore = useAppStore();

const options = computed(() =>
  appStore.localeOptions.map(option => ({ content: option.label, value: option.key }))
);

// TDesign emits the clicked option's value, but older versions pass the whole
// option object or the DOM event. Normalise defensively: a bad value here used
// to write `[object Object]` into <html lang> and blank the navigation.
function handleChange(value: unknown) {
  const raw = value as { value?: unknown } | string | undefined;
  const key = typeof raw === 'string' ? raw : String((raw as { value?: unknown })?.value ?? '');
  if (key !== 'zh-CN' && key !== 'en-US') return;
  appStore.changeLocale(key as App.I18n.LangType);
}
</script>

<template>
  <TDropdown :options="options" :value="appStore.locale" @click="handleChange">
    <TButton variant="text" shape="square" :aria-label="$t('icon.lang')">
      <TranslateIcon />
    </TButton>
  </TDropdown>
</template>
