<script setup lang="ts">
import { reactive, ref } from 'vue';
import type { FormInstanceFunctions, FormRule } from 'tdesign-vue-next';
import { useAuthStore } from '@/store/modules/auth';
import { $t } from '@/locales';

defineOptions({ name: 'LoginPage' });

/**
 * Login page rebuilt against the official TDesign starter layout: left-aligned
 * title block, 400px form column, separated switch links and a bottom
 * copyright, instead of a centred custom card.
 *
 * The authStore.login contract (username / password / optional totp_code ->
 * JWT + CSRF in sessionStorage -> /auth/me -> redirect) is unchanged, so the
 * router guard, RBAC and request interceptors need no changes.
 */

const authStore = useAuthStore();

const formRef = ref<FormInstanceFunctions | null>(null);
const loading = ref(false);
const errorMsg = ref('');
const showTotp = ref(false);

const formData = reactive({
  username: '',
  password: '',
  totp_code: ''
});

const rules: Record<string, FormRule[]> = {
  username: [{ required: true, message: $t('auth.usernameRequired'), type: 'error' }],
  password: [{ required: true, message: $t('auth.passwordRequired'), type: 'error' }]
};

const year = new Date().getFullYear();

async function handleLogin() {
  if (loading.value) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  loading.value = true;
  errorMsg.value = '';

  try {
    await authStore.login(formData.username, formData.password, formData.totp_code || undefined);
  } catch (err: unknown) {
    const errMessage = err instanceof Error ? err.message : $t('auth.loginFailed');
    const lower = errMessage.toLowerCase();
    if (lower.includes('totp') || lower.includes('2fa') || lower.includes('two-factor') || lower.includes('验证码')) {
      showTotp.value = true;
    }
    errorMsg.value = errMessage;
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="login-wrapper">
    <div class="login-container">
      <div class="title-container">
        <div class="login-logo"><TIcon name="logo-github-filled" size="40px" /></div>
        <h1 class="title">{{ $t('system.title') }}</h1>
        <div class="sub-title">
          <span class="tip">{{ $t('system.loginSubtitle') }}</span>
        </div>
      </div>

      <div class="item-container">
        <TForm ref="formRef" :data="formData" :rules="rules" label-align="top" @submit="handleLogin">
          <TFormItem name="username" :label-width="0">
            <TInput
              v-model="formData.username"
              size="large"
              clearable
              autocomplete="username"
              :aria-label="$t('auth.username')"
              :placeholder="$t('auth.username')"
            >
              <template #prefix-icon><TIcon name="user" /></template>
            </TInput>
          </TFormItem>

          <TFormItem name="password" :label-width="0">
            <TInput
              v-model="formData.password"
              size="large"
              type="password"
              autocomplete="current-password"
              :aria-label="$t('auth.password')"
              :placeholder="$t('auth.password')"
              @enter="handleLogin"
            >
              <template #prefix-icon><TIcon name="lock-on" /></template>
            </TInput>
          </TFormItem>

          <TFormItem v-if="showTotp" name="totp_code" :label-width="0">
            <TInput
              v-model="formData.totp_code"
              size="large"
              :aria-label="$t('auth.totpCode')"
              :placeholder="$t('auth.totpCode')"
              @enter="handleLogin"
            >
              <template #prefix-icon><TIcon name="secured" /></template>
            </TInput>
          </TFormItem>

          <TAlert v-if="errorMsg" theme="error" role="alert" :message="errorMsg" class="login-error" />

          <div class="btn-container">
            <TButton theme="primary" size="large" block :loading="loading" type="submit">
              {{ $t('auth.login') }}
            </TButton>
          </div>
        </TForm>
      </div>
    </div>

    <div class="copyright">Copyright © 2024-{{ year }} GoDDI Inc. All Rights Reserved.</div>
  </div>
</template>

<style scoped>
.login-wrapper {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100vh;
  background-color: var(--td-bg-color-page);
  background-image: radial-gradient(ellipse at 100% 0%, var(--td-brand-color-1), transparent 60%);
  background-size: cover;
  background-position: 100%;
}
.login-container {
  position: absolute;
  top: 22%;
  left: 5%;
  min-height: 500px;
}
.login-logo {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: var(--td-radius-large);
  background: var(--td-brand-color);
  color: var(--td-text-color-anti);
}
.title-container .title {
  margin: var(--td-comp-margin-xs) 0 0;
  font: var(--td-font-headline-large);
  color: var(--td-text-color-primary);
}
.title-container .sub-title {
  margin-top: var(--td-comp-margin-xxl);
  font: var(--td-font-body-medium);
  color: var(--td-text-color-secondary);
}
.item-container {
  width: 400px;
  max-width: 100%;
  margin-top: var(--td-comp-margin-xxxxl);
}
.btn-container {
  margin-top: var(--td-comp-margin-xxxxl);
}
.login-error {
  margin-bottom: var(--td-comp-margin-l);
}
.copyright {
  position: absolute;
  left: 5%;
  bottom: 64px;
  font: var(--td-font-body-medium);
  color: var(--td-text-color-secondary);
}
@media (max-width: 768px) {
  .login-container {
    top: 12%;
    left: 6%;
    right: 6%;
  }
  .item-container {
    width: 100%;
  }
  .copyright {
    display: none;
  }
}
@media screen and (height <= 700px) {
  .copyright {
    display: none;
  }
}
</style>
