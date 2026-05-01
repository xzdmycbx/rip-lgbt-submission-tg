<template>
  <AdminLayout>
    <div class="page-head">
      <div>
        <h2 class="page-title">绑定二步验证</h2>
        <p class="page-subtitle">至少绑定一项后才能进入后台。</p>
      </div>
      <div class="actions">
        <button v-if="canFinish" class="button primary" type="button" @click="finish">完成 →</button>
      </div>
    </div>

    <div class="card">
      <h3>动态验证码（TOTP）</h3>
      <p class="card-subtitle">推荐 Authy / 1Password / Google Authenticator。扫描二维码或手动添加密钥后输入当前的 6 位代码完成绑定。</p>

      <template v-if="!totp.ready">
        <span v-if="totpConfirmed" class="badge ok">✓ 已绑定</span>
        <button v-else class="button" type="button" @click="startTOTP" :disabled="busy">生成绑定二维码</button>
      </template>
      <template v-else>
        <div class="field-row" style="align-items: stretch;">
          <div>
            <p class="field-hint">用 App 扫描这个二维码：</p>
            <img v-if="totp.qr" :src="totp.qr" alt="otpauth qr" class="qr-img" />
            <p class="field-hint" style="margin-top:6px;">或手动输入密钥：</p>
            <code class="totp-secret">{{ totp.secret }}</code>
          </div>
          <div>
            <label class="field">
              <span class="label">输入当前 6 位验证码</span>
              <input v-model="totp.code" maxlength="6" inputmode="numeric" pattern="\d{6}" />
            </label>
            <button class="button primary" type="button" @click="confirmTOTP" :disabled="busy">确认绑定</button>
          </div>
        </div>
      </template>
    </div>

    <div class="card">
      <h3>Passkey</h3>
      <p class="card-subtitle">通过设备 / 浏览器内置的 WebAuthn 进行无密码登录。</p>
      <span v-if="hasPasskey" class="badge ok">✓ 已绑定 {{ passkeyCount }} 个 Passkey</span>
      <span v-else class="badge muted">尚未绑定</span>
      <div style="margin-top: 12px;">
        <button class="button" type="button" @click="startPasskey" :disabled="busy">添加 Passkey</button>
      </div>
    </div>

    <p v-if="status" class="status-line" :class="statusTone">{{ status }}</p>
  </AdminLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import AdminLayout from '@/components/AdminLayout.vue';
import { authAPI } from '@/api/client';
import { useAuthStore } from '@/stores/auth';
import { base64urlToBuffer, bufferToBase64url, describePasskeyError } from '@/api/webauthn';

const router = useRouter();
const auth = useAuthStore();

const totp = reactive({ ready: false, secret: '', otpauth: '', qr: '', code: '' });
const status = ref('');
const statusTone = ref<'ok' | 'error' | ''>('');
const busy = ref(false);
const passkeyCount = ref(0);

const totpConfirmed = computed(() => !!auth.admin?.totp_confirmed);
const hasPasskey = computed(() => !!auth.admin?.has_passkey);
const canFinish = computed(() => totpConfirmed.value || hasPasskey.value);

async function refreshState() {
  await auth.refresh(true);
  try {
    const r = await authAPI.passkeyList();
    passkeyCount.value = r.passkeys?.length || 0;
  } catch {
    // ignore
  }
}

onMounted(refreshState);

async function startTOTP() {
  busy.value = true;
  status.value = '';
  try {
    const r = await authAPI.totpBegin();
    totp.secret = r.secret;
    totp.otpauth = r.otpauth;
    totp.qr = `https://api.qrserver.com/v1/create-qr-code/?size=240x240&data=${encodeURIComponent(r.otpauth)}`;
    totp.ready = true;
  } catch (e: any) {
    status.value = e?.response?.data?.error || 'TOTP 初始化失败';
    statusTone.value = 'error';
  } finally {
    busy.value = false;
  }
}

async function confirmTOTP() {
  busy.value = true;
  try {
    await authAPI.totpConfirm(totp.code);
    status.value = 'TOTP 已绑定';
    statusTone.value = 'ok';
    totp.ready = false;
    await refreshState();
  } catch (e: any) {
    status.value = e?.response?.data?.error === 'invalid_code' ? '验证码不正确' : '绑定失败';
    statusTone.value = 'error';
  } finally {
    busy.value = false;
  }
}

async function startPasskey() {
  if (!('credentials' in navigator)) {
    status.value = '当前浏览器不支持 WebAuthn';
    statusTone.value = 'error';
    return;
  }
  busy.value = true;
  status.value = '';
  try {
    const opts: any = await authAPI.passkeyRegisterBegin();
    const publicKey = opts.publicKey || opts;
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
    await authAPI.passkeyRegisterFinish({
      id: cred.id,
      rawId: bufferToBase64url(cred.rawId),
      type: cred.type,
      response: {
        clientDataJSON: bufferToBase64url(att.clientDataJSON),
        attestationObject: bufferToBase64url(att.attestationObject)
      }
    });
    status.value = 'Passkey 已绑定';
    statusTone.value = 'ok';
    await refreshState();
  } catch (e: any) {
    status.value = describePasskeyError(e);
    statusTone.value = 'error';
  } finally {
    busy.value = false;
  }
}

function finish() {
  router.replace('/admin/queue');
}
</script>
