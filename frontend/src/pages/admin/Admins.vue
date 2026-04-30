<template>
  <AdminLayout>
    <h2>管理员</h2>
    <div class="admin-card">
      <h3>添加管理员</h3>
      <p style="color:var(--muted); font-size:.85rem;">填写至少一个标识 (用户名 或 Telegram ID)。密码可空，TG 管理员可通过 bot 一次性链接登录。</p>
      <div class="row">
        <input v-model="form.username" placeholder="用户名（可选）" style="width:auto;" />
        <input v-model="form.telegram_id" type="number" placeholder="Telegram ID（可选）" style="width:auto;" />
        <input v-model="form.display_name" placeholder="显示名" style="width:auto;" />
        <input v-model="form.password" type="password" placeholder="密码（可选）" style="width:auto;" />
        <label class="row" style="gap:.3rem;">
          <input type="checkbox" v-model="form.is_super" />
          <span>超管</span>
        </label>
        <button class="button primary" type="button" @click="create" :disabled="busy">添加</button>
      </div>
      <p v-if="status" style="color:var(--muted); margin-top:.6rem;">{{ status }}</p>
    </div>

    <div class="admin-card">
      <table>
        <thead><tr><th>ID</th><th>名字</th><th>登录方式</th><th>2FA</th><th></th></tr></thead>
        <tbody>
          <tr v-for="a in admins" :key="a.id">
            <td>{{ a.id }}</td>
            <td>{{ a.display_name || a.username || `tg:${a.telegram_id}` }} <small v-if="a.is_super">[super]</small></td>
            <td>
              <span v-if="a.username">密码: {{ a.username }}</span>
              <span v-if="a.telegram_id"> · TG: {{ a.telegram_id }}</span>
            </td>
            <td>
              <span v-if="a.totp_confirmed">TOTP ✓</span>
              <span v-if="a.has_passkey"> · Passkey ✓</span>
              <span v-if="a.must_setup_2fa">（待设置）</span>
            </td>
            <td>
              <button class="button" type="button" v-if="a.telegram_id" @click="issueLink(a.id)">生成 TG 登录链接</button>
              <button class="button" type="button" @click="remove(a.id)" v-if="!a.is_super || a.id !== auth.admin?.id">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-if="lastLink" style="margin-top: 1rem; color: var(--muted); word-break: break-all;">
        临时登录链接 (10 分钟有效)：<a :href="lastLink">{{ lastLink }}</a>
      </p>
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
const lastLink = ref('');
const busy = ref(false);

const form = reactive({
  username: '',
  telegram_id: 0 as number | null,
  display_name: '',
  password: '',
  is_super: false
});

async function load() {
  try {
    const r = await adminAPI.listAdmins();
    admins.value = r.admins || [];
  } catch {
    admins.value = [];
  }
}

async function create() {
  busy.value = true;
  status.value = '';
  try {
    await adminAPI.createAdmin({
      username: form.username,
      telegram_id: form.telegram_id ? Number(form.telegram_id) : 0,
      display_name: form.display_name || form.username,
      password: form.password,
      is_super: form.is_super
    });
    form.username = '';
    form.telegram_id = 0;
    form.display_name = '';
    form.password = '';
    form.is_super = false;
    await load();
  } catch (e: any) {
    status.value = e?.response?.data?.error || '添加失败';
  } finally {
    busy.value = false;
  }
}

async function remove(id: number) {
  if (!confirm('确定删除？')) return;
  try {
    await adminAPI.deleteAdmin(id);
    await load();
  } catch (e: any) {
    status.value = e?.response?.data?.error || '删除失败';
  }
}

async function issueLink(id: number) {
  try {
    const r = await adminAPI.issueLoginLink(id);
    lastLink.value = r.url;
  } catch (e: any) {
    status.value = e?.response?.data?.error || '生成失败';
  }
}

onMounted(load);
</script>
