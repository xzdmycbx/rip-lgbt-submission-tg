<template>
  <AdminLayout>
    <div class="page-head">
      <div>
        <h2 class="page-title">管理员</h2>
        <p class="page-subtitle">添加/移除可登录管理后台的账号；TG 管理员可在机器人里通过 <code>/login</code> 申请一次性登录链接。</p>
      </div>
    </div>

    <div class="card">
      <h3>添加管理员</h3>
      <p class="card-subtitle">至少填写「用户名」或「Telegram ID」之一。密码可选 — TG 管理员可走免密登录链接。</p>
      <div class="field-row">
        <div class="field"><span class="label">用户名（可选）</span><input v-model="form.username" placeholder="alice" /></div>
        <div class="field"><span class="label">Telegram numeric ID（可选）</span><input v-model.number="form.telegram_id" type="number" /></div>
        <div class="field"><span class="label">显示名</span><input v-model="form.display_name" placeholder="显示在后台" /></div>
        <div class="field"><span class="label">初始密码（可选）</span><input v-model="form.password" type="password" autocomplete="new-password" /></div>
      </div>
      <label class="switch section-h3">
        <input type="checkbox" v-model="form.is_super" />
        <span>授予超级管理员权限</span>
      </label>
      <div class="form-actions">
        <button class="button primary" type="button" @click="create" :disabled="busy">添加</button>
        <span v-if="status" class="status-line" :class="statusTone">{{ status }}</span>
      </div>
    </div>

    <div class="card">
      <h3>已有管理员</h3>
      <table class="data">
        <thead>
          <tr><th>ID</th><th>名字</th><th>登录方式</th><th>2FA</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="a in admins" :key="a.id">
            <td><code>#{{ a.id }}</code></td>
            <td>
              {{ a.display_name || a.username || `tg:${a.telegram_id}` }}
              <span v-if="a.is_super" class="badge info">超管</span>
            </td>
            <td>
              <span v-if="a.username" class="badge muted">密码: {{ a.username }}</span>
              <span v-if="a.telegram_id" class="badge muted">TG: {{ a.telegram_id }}</span>
            </td>
            <td>
              <span v-if="a.totp_confirmed" class="badge ok">TOTP</span>
              <span v-if="a.has_passkey" class="badge ok">Passkey</span>
              <span v-if="a.must_setup_2fa" class="badge warn">待设置</span>
            </td>
            <td class="row-actions">
              <button class="button sm" v-if="a.telegram_id" @click="issueLink(a.id)">生成 TG 登录链接</button>
              <button class="button danger sm" v-if="canDelete(a)" @click="remove(a.id)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-if="lastLink" class="card tight section-h3">
        <h3>新生成的临时登录链接</h3>
        <p class="card-subtitle">10 分钟内有效，只能使用一次。</p>
        <input :value="lastLink" readonly @focus="($event.target as HTMLInputElement).select()" />
      </div>
    </div>
  </AdminLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import AdminLayout from '@/components/AdminLayout.vue';
import { adminAPI } from '@/api/client';
import { useAuthStore } from '@/stores/auth';

const auth = useAuthStore();
const admins = ref<any[]>([]);
const status = ref('');
const statusTone = ref<'ok' | 'error' | ''>('');
const lastLink = ref('');
const busy = ref(false);

const form = reactive({
  username: '',
  telegram_id: 0 as number,
  display_name: '',
  password: '',
  is_super: false
});

async function load() {
  const r = await adminAPI.listAdmins();
  admins.value = r.admins || [];
}

onMounted(load);

async function create() {
  busy.value = true;
  status.value = '';
  try {
    if (!form.username && !form.telegram_id) {
      throw new Error('请至少填写用户名或 Telegram ID');
    }
    await adminAPI.createAdmin({
      username: form.username,
      telegram_id: form.telegram_id ? Number(form.telegram_id) : 0,
      display_name: form.display_name || form.username,
      password: form.password,
      is_super: form.is_super
    });
    Object.assign(form, { username: '', telegram_id: 0, display_name: '', password: '', is_super: false });
    setStatus('已添加', 'ok');
    await load();
  } catch (e: any) {
    setStatus(e?.message || e?.response?.data?.error || '添加失败', 'error');
  } finally {
    busy.value = false;
  }
}

async function remove(id: number) {
  if (!confirm('确定删除这个管理员？')) return;
  try {
    await adminAPI.deleteAdmin(id);
    await load();
  } catch (e: any) {
    setStatus(e?.response?.data?.error || '删除失败', 'error');
  }
}

async function issueLink(id: number) {
  try {
    const r = await adminAPI.issueLoginLink(id);
    lastLink.value = r.url;
  } catch (e: any) {
    setStatus(e?.response?.data?.error || '生成失败', 'error');
  }
}

function canDelete(a: any): boolean {
  if (a.id === auth.admin?.id) return false;
  return true;
}

function setStatus(t: string, tone: 'ok' | 'error') {
  status.value = t;
  statusTone.value = tone;
}
</script>
