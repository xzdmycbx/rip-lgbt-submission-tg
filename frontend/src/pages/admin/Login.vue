<template>
  <div class="admin-login">
    <form @submit.prevent="submit">
      <h1 style="font-family: var(--serif);">登录管理后台</h1>
      <p style="color: var(--muted); margin: 0;">勿忘我 · rip.lgbt</p>
      <label>
        <span>用户名</span>
        <input v-model="form.username" autocomplete="username" required />
      </label>
      <label>
        <span>密码</span>
        <input v-model="form.password" type="password" autocomplete="current-password" required />
      </label>
      <label v-if="needTOTP">
        <span>动态验证码（6 位）</span>
        <input v-model="form.totp" maxlength="6" inputmode="numeric" pattern="\d{6}" required />
      </label>
      <p v-if="error" style="color:#f5a9b8;margin:0;">{{ error }}</p>
      <button class="button primary" type="submit" :disabled="busy">登录</button>

      <hr style="border: none; border-top: 1px solid var(--line); margin: 0.6rem 0;" />

      <button class="button" type="button" @click="passwordlessLogin" :disabled="busy">
        🔐 直接使用 Passkey 登录
      </button>
      <p style="font-size:.85rem;color:var(--muted);margin:0;">
        TG 管理员请通过机器人申请一次性登录链接。
      </p>
    </form>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { authAPI } from '@/api/client';
import { useAuthStore } from '@/stores/auth';
import '@/styles/admin.css';

const router = useRouter();
const route = useRoute();
const auth = useAuthStore();
const form = reactive({ username: '', password: '', totp: '' });
const needTOTP = ref(false);
const error = ref('');
const busy = ref(false);

onMounted(async () => {
  // Token-based TG login
  const token = route.query.token as string | undefined;
  if (token) {
    busy.value = true;
    try {
      const r = await authAPI.loginTG(token);
      if (r.ok) {
        await auth.refresh();
        if (r.need?.must_setup_2fa) {
          router.replace('/admin/setup-2fa');
        } else {
          router.replace('/admin/queue');
        }
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
    if (r.need?.totp) {
      needTOTP.value = true;
      return;
    }
    if (r.ok) {
      await auth.refresh();
      if (r.need?.must_setup_2fa) {
        router.replace('/admin/setup-2fa');
      } else {
        router.replace('/admin/queue');
      }
    } else {
      error.value = r.error || '登录失败';
    }
  } catch (e: any) {
    error.value = e?.response?.data?.error || '登录失败';
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
      await auth.refresh();
      router.replace('/admin/queue');
    } else {
      error.value = '登录失败';
    }
  } catch (e: any) {
    error.value = e?.response?.data?.error || e?.message || 'Passkey 登录失败';
  } finally {
    busy.value = false;
  }
}

function base64urlToBuffer(s: string): ArrayBuffer {
  const pad = '='.repeat((4 - (s.length % 4)) % 4);
  const b64 = (s + pad).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(b64);
  const buf = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) buf[i] = raw.charCodeAt(i);
  return buf.buffer;
}
function bufferToBase64url(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let bin = '';
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}
</script>
