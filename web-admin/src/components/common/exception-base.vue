<script lang="ts" setup>
import { computed } from 'vue';
import { useRouterPush } from '@/hooks/common/router';
import { $t } from '@/locales';

defineOptions({ name: 'ExceptionBase' });

type ExceptionType = '403' | '404' | '500';

interface Props {
  /**
   * Exception type
   *
   * - 403: no permission
   * - 404: not found
   * - 500: service error
   */
  type: ExceptionType;
}

const props = defineProps<Props>();

const { routerPushByKey } = useRouterPush();

// The previous markup relied on flex-col-center / text-400px utilities that
// this project never defined, so the page collapsed to an empty box. This
// version is self-contained: theme tokens only, no missing utility classes.
const titleMap: Record<ExceptionType, string> = {
  '403': '403',
  '404': '404',
  '500': '500'
};

const descMap: Record<ExceptionType, string> = {
  '403': 'common.exception403',
  '404': 'common.exception404',
  '500': 'common.exception500'
};

const code = computed(() => titleMap[props.type]);
const description = computed(() => $t(descMap[props.type] as any));
</script>

<template>
  <div class="exception-wrap">
    <div class="exception-inner">
      <span class="exception-code">{{ code }}</span>
      <p class="exception-desc">{{ description }}</p>
      <TButton theme="primary" variant="outline" @click="routerPushByKey('root')">
        {{ $t('common.backToHome') }}
      </TButton>
    </div>
  </div>
</template>

<style scoped>
.exception-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: calc(100dvh - 200px);
  padding: var(--td-comp-paddingTB-xxl) var(--td-comp-paddingLR-xl);
}
.exception-inner {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--td-comp-margin-l);
  text-align: center;
}
.exception-code {
  font: var(--td-font-headline-extra-large);
  color: var(--td-brand-color);
  letter-spacing: 4px;
  line-height: 1;
}
.exception-desc {
  margin: 0;
  font: var(--td-font-body-large);
  color: var(--td-text-color-secondary);
}
</style>
