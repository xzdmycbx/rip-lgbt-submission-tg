<template>
  <div class="admin-app admin-login-page">
    <div class="admin-login-card">
      <div>
        <h1>勿忘我 · 后台</h1>
        <p class="lede">使用账号密码登录。如已绑定 Passkey，可直接走免密登录。</p>
      </div>

      <form @submit.prevent="submit">
        <div class="field">
          <span class="label">用户名</span>
          <input v-model="form.username" autocomplete="username" required autofocus />
        </div>

        <div class="field">
          <span class="label">密码</span>
          <input v-model="form.password" type="password" autocomplete="current-password" required />
        </div>

        <p v-if="error" class="status-line error">{{ error }}</p>

        <button class="button primary" type="submit" :disabled="busy">继续</button>
      </form>

      <div class="divider">或</div>

      <button class="button" type="button" @click="passwordlessLogin" :disabled="busy">
        🔐 使用 Passkey 直接登录
      </button>

      <p class="small-link">
        TG 管理员可在机器人发送 <code>/login</code> 获取一次性登录链接。
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { authAPI } from '@/api/client';
import { useAuthStore } from '@/stores/auth';
import { base64urlToBuffer, bufferToBase64url, describePasskeyError } from '@/api/webauthn';
import '@/styles/admin.css';

const router = useRouter();
const route = useRoute();
const auth = useAuthStore();
const form = reactive({ username: '', password: '' });
const error = ref('');
const busy = ref(false);

const next = (route.query.next as string | undefined) || '/admin/queue';

onMounted(async () => {
  // TG token-based login.
  const token = route.query.token as string | undefined;
  if (token) {
    busy.value = true;
    try {
      const r = await authAPI.loginTG(token);
      if (r.ok) {
        await auth.refresh(true);
        if (r.need?.must_setup_2fa) router.replace('/admin/setup-2fa');
        else router.replace(next);
        return;
      }
      error.value = r.error || '登录链接无效或已过期';
    } catch (e: any) {
      error.value = e?.response?.data?.error || '登录失败';
    } finally {
      busy.value = false;
    }
  }
});

async function submit() {
  busy.value = true;
  error.value = '';
  try {
    const r = await authAPI.login(form);
    if (r.need?.totp && (r as any).pending_token) {
      router.replace({
        path: '/admin/login/totp',
        query: { token: (r as any).pending_token, next }
      });
      return;
    }
    if (r.ok) {
      await auth.refresh(true);
      if (r.need?.must_setup_2fa) router.replace('/admin/setup-2fa');
      else router.replace(next);
      return;
    }
    error.value = readableError(r.error) || '登录失败';
  } catch (e: any) {
    error.value = readableError(e?.response?.data?.error) || '登录失败';
  } finally {
    busy.value = false;
  }
}

async function passwordlessLogin() {
  if (!('credentials' in navigator)) {
    error.value = '当前浏览器不支持 WebAuthn。';
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    const begin = await authAPI.passkeyDiscoverableBegin();
    const publicKey = (begin.options.publicKey || begin.options) as any;
    publicKey.challenge = base64urlToBuffer(publicKey.challenge);
    if (publicKey.allowCredentials) {
      publicKey.allowCredentials = publicKey.allowCredentials.map((c: any) => ({
        ...c,
        id: base64urlToBuffer(c.id)
      }));
    }
    const cred = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential;
    const ar = cred.response as AuthenticatorAssertionResponse;
    const response = {
      id: cred.id,
      rawId: bufferToBase64url(cred.rawId),
      type: cred.type,
      response: {
        clientDataJSON: bufferToBase64url(ar.clientDataJSON),
        authenticatorData: bufferToBase64url(ar.authenticatorData),
        signature: bufferToBase64url(ar.signature),
        userHandle: ar.userHandle ? bufferToBase64url(ar.userHandle) : null
      }
    };
    const r = await authAPI.passkeyDiscoverableFinish(begin.challenge_token, response);
    if (r.ok) {
      await auth.refresh(true);
      router.replace(next);
    } else {
      error.value = '登录失败';
    }
  } catch (e: any) {
    error.value = describePasskeyError(e);
  } finally {
    busy.value = false;
  }
}

function readableError(code?: string): string {
  switch (code) {
    case 'invalid_credentials': return '用户名或密码不正确';
    case 'missing_credentials': return '请填写用户名和密码';
    case 'invalid_token': return '登录链接无效或已使用';
    default: return code || '';
  }
}
</script>
