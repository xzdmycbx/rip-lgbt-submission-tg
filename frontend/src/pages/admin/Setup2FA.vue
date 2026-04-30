<template>
  <AdminLayout>
    <h2>设置二步验证</h2>
    <div class="admin-card">
      <p>登录后请绑定 TOTP 与 Passkey。两项都绑定后，账号会自动解除「必须完成 2FA」限制。</p>

      <h3 style="font-size:1.05rem; margin-top:1.2rem;">动态验证码 (TOTP)</h3>
      <div v-if="!totpReady">
        <button class="button primary" type="button" @click="startTOTP" :disabled="busy">生成绑定二维码</button>
      </div>
      <div v-else>
        <p style="font-size:.85rem; color:var(--muted);">用 Authy / 1Password / Google Authenticator 等扫码或手动添加，输入显示的 6 位验证码确认绑定。</p>
        <div class="row">
          <code style="word-break: break-all;">{{ otpauth }}</code>
        </div>
        <label>
          <span>当前 6 位验证码</span>
          <input v-model="totpCode" maxlength="6" inputmode="numeric" pattern="\d{6}" />
        </label>
        <button class="button" type="button" @click="confirmTOTP" :disabled="busy">确认绑定</button>
      </div>

      <h3 style="font-size:1.05rem; margin-top:1.6rem;">Passkey</h3>
      <p>
        Passkey 绑定流程依赖浏览器 WebAuthn 原生 API。如果当前浏览器不支持，请改用支持的浏览器或登录后再绑定。
      </p>
      <button class="button" type="button" @click="startPasskey" :disabled="busy">注册 Passkey</button>

      <p v-if="status" style="margin-top:1.2rem;color:var(--muted);">{{ status }}</p>

      <div style="margin-top:1.6rem;">
        <button class="button primary" type="button" @click="finish">完成</button>
      </div>
    </div>
  </AdminLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import AdminLayout from '@/components/AdminLayout.vue';
import { authAPI } from '@/api/client';
import { useAuthStore } from '@/stores/auth';

const router = useRouter();
const auth = useAuthStore();

const busy = ref(false);
const status = ref('');
const totpReady = ref(false);
const otpauth = ref('');
const totpCode = ref('');

async function startTOTP() {
  busy.value = true;
  status.value = '';
  try {
    const r = await authAPI.totpBegin();
    otpauth.value = r.otpauth;
    totpReady.value = true;
  } catch (e: any) {
    status.value = e?.response?.data?.error || 'TOTP 初始化失败';
  } finally {
    busy.value = false;
  }
}

async function confirmTOTP() {
  if (!totpCode.value) return;
  busy.value = true;
  try {
    await authAPI.totpConfirm(totpCode.value);
    status.value = 'TOTP 已绑定。';
    await auth.refresh();
  } catch (e: any) {
    status.value = e?.response?.data?.error || '验证码不正确';
  } finally {
    busy.value = false;
  }
}

async function startPasskey() {
  if (!('credentials' in navigator)) {
    status.value = '当前浏览器不支持 WebAuthn。';
    return;
  }
  busy.value = true;
  try {
    const options: any = await authAPI.passkeyRegisterBegin();
    const publicKey = options.publicKey || options;
    publicKey.challenge = base64urlToBuffer(publicKey.challenge);
    publicKey.user.id = base64urlToBuffer(publicKey.user.id);
    if (publicKey.excludeCredentials) {
      publicKey.excludeCredentials = publicKey.excludeCredentials.map((c: any) => ({
        ...c,
        id: base64urlToBuffer(c.id)
      }));
    }
    const cred = (await navigator.credentials.create({ publicKey })) as PublicKeyCredential;
    const att = cred.response as AuthenticatorAttestationResponse;
    const body = {
      id: cred.id,
      rawId: bufferToBase64url(cred.rawId),
      type: cred.type,
      response: {
        clientDataJSON: bufferToBase64url(att.clientDataJSON),
        attestationObject: bufferToBase64url(att.attestationObject)
      }
    };
    await authAPI.passkeyRegisterFinish(body);
    status.value = 'Passkey 已绑定。';
    await auth.refresh();
  } catch (e: any) {
    status.value = e?.message || e?.response?.data?.error || 'Passkey 注册失败';
  } finally {
    busy.value = false;
  }
}

function finish() {
  router.replace('/admin/queue');
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
