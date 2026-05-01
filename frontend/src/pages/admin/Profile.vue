<template>
  <AdminLayout>
    <div class="page-head">
      <div>
        <h2 class="page-title">我的账号</h2>
        <p class="page-subtitle">在这里管理你自己的资料、密码、二步验证与 Passkey。</p>
      </div>
    </div>

    <!-- Account info -->
    <section class="card">
      <h3>账号信息</h3>
      <p class="card-subtitle">
        显示名可以随时修改；Telegram ID 也可自行更换。<br />
        用户名用于密码登录，<strong>仅在首次设置时可填写</strong>，设定后不能再改（如需更名请联系超级管理员重建账号）。
      </p>

      <div class="field-row">
        <div class="field">
          <span class="label">显示名</span>
          <input v-model="profileForm.display_name" maxlength="64" placeholder="例如：Alex" />
        </div>

        <div class="field">
          <span class="label">用户名</span>
          <input
            v-if="!hasUsername"
            v-model="profileForm.username"
            maxlength="32"
            placeholder="3-32 位 a-z 0-9 _ -"
            autocomplete="username"
          />
          <input v-else :value="auth.admin?.username" disabled />
          <p class="field-hint">
            <span v-if="!hasUsername">设置后即可使用账号密码登录。一经设置不可修改。</span>
            <span v-else>已锁定。</span>
          </p>
        </div>

        <div class="field">
          <span class="label">Telegram numeric ID</span>
          <input v-model.number="profileForm.telegram_id" type="number" placeholder="例如：123456789" />
          <p class="field-hint">私聊 <a href="https://t.me/userinfobot" target="_blank" rel="noopener">@userinfobot</a> 可查看自己的 numeric ID。修改后即可让机器人识别为本人。</p>
        </div>
      </div>

      <div class="form-actions">
        <button class="button primary" type="button" @click="saveProfile" :disabled="busy">保存账号信息</button>
        <span v-if="profileMsg" class="status-line" :class="profileTone">{{ profileMsg }}</span>
      </div>
    </section>

    <!-- Password -->
    <section class="card">
      <h3>{{ hasPassword ? '修改密码' : '设置密码' }}</h3>
      <p class="card-subtitle">
        <span v-if="!hasUsername">请先在上方为账号设置一个用户名后，再设置密码。</span>
        <span v-else-if="!hasPassword">尚未设置密码。设置后即可在登录页用账号密码登录。</span>
        <span v-else>修改密码需要输入当前密码以确认身份。</span>
      </p>

      <div class="field-row" v-if="hasUsername">
        <div class="field" v-if="hasPassword">
          <span class="label">当前密码</span>
          <input v-model="pwForm.current" type="password" autocomplete="current-password" />
        </div>
        <div class="field">
          <span class="label">新密码</span>
          <input v-model="pwForm.next" type="password" autocomplete="new-password" />
          <p class="field-hint">至少 8 个字符。</p>
        </div>
        <div class="field">
          <span class="label">再次输入新密码</span>
          <input v-model="pwForm.confirm" type="password" autocomplete="new-password" />
        </div>
      </div>

      <div class="form-actions" v-if="hasUsername">
        <button class="button primary" type="button" @click="savePassword" :disabled="busy">{{ hasPassword ? '更新密码' : '设置密码' }}</button>
        <span v-if="pwMsg" class="status-line" :class="pwTone">{{ pwMsg }}</span>
      </div>
    </section>

    <!-- TOTP -->
    <section class="card">
      <h3>动态验证码（TOTP）</h3>
      <p class="card-subtitle">推荐 Authy / 1Password / Google Authenticator。</p>

      <template v-if="totpConfirmed">
        <span class="badge ok">✓ 已绑定</span>
        <div class="field" style="max-width: 240px; margin-top: 14px;">
          <span class="label">解绑前请输入当前验证码</span>
          <input v-model="disableCode" maxlength="6" inputmode="numeric" pattern="\d{6}" placeholder="6 位代码" />
        </div>
        <div class="form-actions">
          <button class="button danger" type="button" @click="disableTOTP" :disabled="busy || !disableCode">解绑 TOTP</button>
        </div>
      </template>

      <template v-else-if="totpReady">
        <div class="field-row" style="align-items: stretch;">
          <div class="field">
            <span class="label">扫描二维码</span>
            <img v-if="totpQR" :src="totpQR" alt="otpauth qr" class="qr-img" />
            <p class="field-hint">或在 App 中手动添加密钥：</p>
            <code class="totp-secret">{{ totpSecret }}</code>
          </div>
          <div class="field">
            <span class="label">输入当前 6 位验证码完成绑定</span>
            <input v-model="totpCode" maxlength="6" inputmode="numeric" pattern="\d{6}" />
            <div class="form-actions">
              <button class="button primary" type="button" @click="confirmTOTP" :disabled="busy || !totpCode">确认绑定</button>
              <button class="button ghost" type="button" @click="totpReady = false">取消</button>
            </div>
          </div>
        </div>
      </template>

      <template v-else>
        <span class="badge muted">尚未绑定</span>
        <div class="form-actions">
          <button class="button" type="button" @click="startTOTP" :disabled="busy">绑定 TOTP</button>
        </div>
      </template>

      <p v-if="totpMsg" class="status-line" :class="totpTone">{{ totpMsg }}</p>
    </section>

    <!-- Passkey -->
    <section class="card">
      <h3>Passkey</h3>
      <p class="card-subtitle">无密码登录的安全凭据。同一账号可以同时绑定多个设备。</p>

      <table v-if="passkeys.length" class="data" style="margin-bottom: 14px;">
        <thead><tr><th>ID</th><th>传输方式</th><th>注册时间</th><th></th></tr></thead>
        <tbody>
          <tr v-for="p in passkeys" :key="p.id">
            <td><code>#{{ p.id }}</code></td>
            <td>{{ formatTransports(p.transports) }}</td>
            <td>{{ formatTime(p.created_at) }}</td>
            <td class="row-actions">
              <button class="button danger sm" type="button" @click="removePasskey(p.id)" :disabled="busy">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="empty-state" style="text-align: left; padding: 0 0 8px;">尚未绑定任何 Passkey。</p>

      <div class="form-actions">
        <button class="button primary" type="button" @click="addPasskey" :disabled="busy">+ 添加 Passkey</button>
        <span v-if="passkeyMsg" class="status-line" :class="passkeyTone">{{ passkeyMsg }}</span>
      </div>
    </section>
  </AdminLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import AdminLayout from '@/components/AdminLayout.vue';
import { authAPI } from '@/api/client';
import { useAuthStore } from '@/stores/auth';
import { base64urlToBuffer, bufferToBase64url, describePasskeyError } from '@/api/webauthn';

const auth = useAuthStore();
const passkeys = ref<{ id: number; transports: string; created_at: string }[]>([]);

// Profile form
const profileForm = reactive({
  display_name: '',
  username: '',
  telegram_id: 0 as number
});
const profileMsg = ref('');
const profileTone = ref<'ok' | 'error' | ''>('');

// Password form
const pwForm = reactive({ current: '', next: '', confirm: '' });
const pwMsg = ref('');
const pwTone = ref<'ok' | 'error' | ''>('');

// TOTP
const totpReady = ref(false);
const totpSecret = ref('');
const totpQR = ref('');
const totpCode = ref('');
const disableCode = ref('');
const totpMsg = ref('');
const totpTone = ref<'ok' | 'error' | ''>('');

// Passkey
const passkeyMsg = ref('');
const passkeyTone = ref<'ok' | 'error' | ''>('');

const busy = ref(false);

const hasUsername = computed(() => !!auth.admin?.username);
const hasPassword = computed(() => !!auth.admin?.username); // username + password coupled in current model
const totpConfirmed = computed(() => !!auth.admin?.totp_confirmed);

async function refreshState() {
  await auth.refresh(true);
  if (auth.admin) {
    profileForm.display_name = auth.admin.display_name || '';
    profileForm.username = auth.admin.username || '';
    profileForm.telegram_id = auth.admin.telegram_id || 0;
  }
  try {
    const r = await authAPI.passkeyList();
    passkeys.value = r.passkeys || [];
  } catch {
    passkeys.value = [];
  }
}

onMounted(refreshState);

async function saveProfile() {
  busy.value = true;
  profileMsg.value = '';
  try {
    const patch: any = { display_name: profileForm.display_name };
    if (!hasUsername.value && profileForm.username) {
      patch.username = profileForm.username;
    }
    if (Number(profileForm.telegram_id) !== (auth.admin?.telegram_id || 0)) {
      patch.telegram_id = profileForm.telegram_id ? Number(profileForm.telegram_id) : null;
    }
    await authAPI.updateMe(patch);
    setProfile('已保存', 'ok');
    await refreshState();
  } catch (e: any) {
    setProfile(e?.response?.data?.message || e?.response?.data?.error || '保存失败', 'error');
  } finally {
    busy.value = false;
  }
}

async function savePassword() {
  if (pwForm.next !== pwForm.confirm) {
    setPw('两次输入的新密码不一致', 'error');
    return;
  }
  if (pwForm.next.length < 8) {
    setPw('新密码至少 8 个字符', 'error');
    return;
  }
  busy.value = true;
  pwMsg.value = '';
  try {
    await authAPI.changePassword(pwForm.current, pwForm.next);
    setPw(hasPassword.value ? '密码已更新' : '密码已设置', 'ok');
    pwForm.current = '';
    pwForm.next = '';
    pwForm.confirm = '';
  } catch (e: any) {
    setPw(e?.response?.data?.message || e?.response?.data?.error || '操作失败', 'error');
  } finally {
    busy.value = false;
  }
}

async function startTOTP() {
  busy.value = true;
  totpMsg.value = '';
  try {
    const r = await authAPI.totpBegin();
    totpSecret.value = r.secret;
    totpQR.value = `https://api.qrserver.com/v1/create-qr-code/?size=240x240&data=${encodeURIComponent(r.otpauth)}`;
    totpReady.value = true;
  } catch (e: any) {
    setTotp(e?.response?.data?.error || 'TOTP 初始化失败', 'error');
  } finally {
    busy.value = false;
  }
}

async function confirmTOTP() {
  busy.value = true;
  try {
    await authAPI.totpConfirm(totpCode.value);
    setTotp('TOTP 已绑定', 'ok');
    totpReady.value = false;
    totpCode.value = '';
    await refreshState();
  } catch (e: any) {
    setTotp(e?.response?.data?.error === 'invalid_code' ? '验证码不正确' : '绑定失败', 'error');
  } finally {
    busy.value = false;
  }
}

async function disableTOTP() {
  if (!confirm('确认解绑 TOTP？')) return;
  busy.value = true;
  try {
    await authAPI.totpDisable(disableCode.value);
    setTotp('TOTP 已解绑', 'ok');
    disableCode.value = '';
    await refreshState();
  } catch (e: any) {
    setTotp(e?.response?.data?.error === 'invalid_code' ? '验证码不正确' : '解绑失败', 'error');
  } finally {
    busy.value = false;
  }
}

async function addPasskey() {
  if (!('credentials' in navigator)) {
    setPasskey('当前浏览器不支持 WebAuthn', 'error');
    return;
  }
  busy.value = true;
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
    setPasskey('Passkey 已添加', 'ok');
    await refreshState();
  } catch (e: any) {
    setPasskey(describePasskeyError(e), 'error');
  } finally {
    busy.value = false;
  }
}

async function removePasskey(id: number) {
  if (!confirm(`确认删除 Passkey #${id}？`)) return;
  busy.value = true;
  try {
    await authAPI.passkeyDelete(id);
    setPasskey('Passkey 已删除', 'ok');
    await refreshState();
  } catch (e: any) {
    setPasskey(e?.response?.data?.error || '删除失败', 'error');
  } finally {
    busy.value = false;
  }
}

function formatTransports(t: string): string {
  if (!t) return '—';
  try {
    const arr = JSON.parse(t);
    return Array.isArray(arr) && arr.length ? arr.join(', ') : '—';
  } catch {
    return t;
  }
}

function formatTime(s: string): string {
  if (!s) return '';
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString('zh-CN');
}

function setProfile(t: string, tone: 'ok' | 'error') {
  profileMsg.value = t;
  profileTone.value = tone;
}
function setPw(t: string, tone: 'ok' | 'error') {
  pwMsg.value = t;
  pwTone.value = tone;
}
function setTotp(t: string, tone: 'ok' | 'error') {
  totpMsg.value = t;
  totpTone.value = tone;
}
function setPasskey(t: string, tone: 'ok' | 'error') {
  passkeyMsg.value = t;
  passkeyTone.value = tone;
}
</script>

<style scoped>
/* Profile-specific tweaks. Generic helpers live in admin.css. */
</style>
