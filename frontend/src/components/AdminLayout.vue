<template>
  <div class="admin-shell">
    <aside class="admin-sidebar">
      <h1>勿忘我 · 管理</h1>
      <p class="muted" style="font-size:.82rem;color:var(--muted);">
        {{ auth.admin?.display_name || auth.admin?.username || 'admin' }}
        <span v-if="auth.admin?.is_super">（超管）</span>
      </p>
      <nav>
        <RouterLink to="/admin/queue">待审投稿</RouterLink>
        <RouterLink to="/admin/admins" v-if="auth.admin?.is_super">管理员</RouterLink>
        <RouterLink to="/admin/settings" v-if="auth.admin?.is_super">设置</RouterLink>
        <a href="#" @click.prevent="logout">退出</a>
      </nav>
    </aside>
    <main class="admin-main">
      <slot />
    </main>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '@/stores/auth';
import '@/styles/admin.css';

const auth = useAuthStore();
const router = useRouter();

onMounted(async () => {
  await auth.refresh();
  if (!auth.admin) {
    router.replace('/admin/login');
    return;
  }
  if (auth.admin.must_setup_2fa && router.currentRoute.value.name !== 'admin-setup-2fa') {
    router.replace('/admin/setup-2fa');
  }
});

async function logout() {
  await auth.logout();
  router.replace('/admin/login');
}
</script>
