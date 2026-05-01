<template>
  <div class="admin-app admin-login-page">
    <div class="admin-login-card">
      <div>
        <h1>动态验证码</h1>
        <p class="lede">为了保护账号安全，请输入验证器 App 中的 6 位动态验证码。</p>
      </div>

      <div class="factor-card">
        <div class="factor-icon" aria-hidden="true">🔐</div>
        <div>
          <div style="font-weight:600;">基于 TOTP 的二步验证</div>
          <div style="color: var(--text-muted); font-size: 12.5px;">在 Authy / 1Password / Google Authenticator 等 App 中查看。</div>
        </div>
      </div>

      <form @submit.prevent="submit">
        <div class="field otp-field">
          <span class="label">动态验证码</span>
          <input
            v-model="code"
            inputmode="numeric"
            pattern="\d{6}"
            maxlength="6"
            autocomplete="one-time-code"
            required
            autofocus
            placeholder="······"
          />
        </div>
        <p v-if="error" class="status-line error">{{ error }}</p>
        <div style="display:flex; gap:8px;">
          <button class="button" type="button" @click="back" :disabled="busy">返回</button>
          <button class="button primary" type="submit" :disabled="busy" style="flex:1;">登录</button>
        </div>
      </form>

      <p class="small-link">如果设备不可用，请联系超级管理员重置 2FA。</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { authAPI, api } from '@/api/client';
import { useAuthStore } from '@/stores/auth';
import '@/styles/admin.css';

const router = useRouter();
const route = useRoute();
const auth = useAuthStore();

const code = ref('');
const error = ref('');
const busy = ref(false);
const pendingToken = ref((route.query.token as string) || '');
const next = (route.query.next as string) || '/admin/queue';

onMounted(() => {
  if (!pendingToken.value) {
    router.replace('/admin/login');
  }
});

async function submit() {
  busy.value = true;
  error.value = '';
  try {
    const r = await api
      .post('/api/auth/login/totp', {
        pending_token: pendingToken.value,
        code: code.value
      })
      .then((x) => x.data);
    if (r.ok) {
      await auth.refresh(true);
      if (r.need?.must_setup_2fa) router.replace('/admin/setup-2fa');
      else router.replace(next);
      return;
    }
    if (r.pending_token) {
      pendingToken.value = r.pending_token;
    }
    error.value = readableError(r.error) || '验证码不正确';
  } catch (e: any) {
    const data = e?.response?.data;
    if (data?.pending_token) pendingToken.value = data.pending_token;
    error.value = readableError(data?.error) || '验证失败';
  } finally {
    busy.value = false;
    code.value = '';
  }
}

function back() {
  router.replace('/admin/login');
}

function readableError(code?: string): string {
  switch (code) {
    case 'invalid_totp': return '验证码不正确，请再试一次';
    case 'expired_or_unknown_token': return '会话已过期，请重新输入密码';
    case 'totp_unbound': return '该账号尚未绑定 TOTP';
    default: return code || '';
  }
}

void authAPI;
</script>
