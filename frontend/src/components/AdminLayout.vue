<template>
  <div class="admin-app">
    <div class="admin-shell">
      <aside class="admin-sidebar">
        <RouterLink class="brand" to="/admin/queue">
          <span class="brand-mark" aria-hidden="true">勿</span>
          <span>
            <strong>勿忘我</strong>
            <small>ADMIN</small>
          </span>
        </RouterLink>

        <p class="who" v-if="auth.admin">
          <strong>{{ auth.admin.display_name || auth.admin.username || `tg:${auth.admin.telegram_id}` }}</strong>
          <span v-if="auth.admin.is_super">超级管理员</span>
          <span v-else>管理员</span>
        </p>

        <nav>
          <span class="section-label">投稿</span>
          <RouterLink to="/admin/queue">
            <span class="icon" aria-hidden="true">📥</span>
            <span>待审投稿</span>
          </RouterLink>
          <RouterLink to="/admin/memorials">
            <span class="icon" aria-hidden="true">📜</span>
            <span>已发布稿件</span>
          </RouterLink>

          <span class="section-label">账号</span>
          <RouterLink to="/admin/profile">
            <span class="icon" aria-hidden="true">🔐</span>
            <span>我的安全</span>
          </RouterLink>

          <template v-if="auth.admin?.is_super">
            <span class="section-label">超管</span>
            <RouterLink to="/admin/admins">
              <span class="icon" aria-hidden="true">👥</span>
              <span>管理员</span>
            </RouterLink>
            <RouterLink to="/admin/settings">
              <span class="icon" aria-hidden="true">⚙️</span>
              <span>设置</span>
            </RouterLink>
          </template>
        </nav>

        <div class="footer-actions">
          <button class="button ghost sm" type="button" @click="logout">退出登录</button>
        </div>
      </aside>

      <main class="admin-main">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useAuthStore } from '@/stores/auth';
import '@/styles/admin.css';

const auth = useAuthStore();
const router = useRouter();
const route = useRoute();

onMounted(async () => {
  await auth.refresh(true);
  if (!auth.admin) {
    router.replace({ path: '/admin/login', query: { next: route.fullPath } });
    return;
  }
  if (auth.admin.must_setup_2fa && route.name !== 'admin-setup-2fa') {
    router.replace('/admin/setup-2fa');
  }
});

async function logout() {
  await auth.logout();
  router.replace('/admin/login');
}
</script>
