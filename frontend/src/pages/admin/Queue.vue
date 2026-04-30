<template>
  <AdminLayout>
    <h2>待审投稿</h2>
    <div class="admin-card" v-if="loading">加载中…</div>
    <div class="admin-card" v-else-if="!drafts.length">当前没有需要审核的投稿。</div>
    <div class="admin-card" v-else>
      <table>
        <thead>
          <tr><th>展示名</th><th>条目ID</th><th>提交人</th><th>提交时间</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="d in drafts" :key="d.id">
            <td>{{ d.display_name || '(未填写)' }}</td>
            <td><code>{{ d.entry_id || '—' }}</code></td>
            <td>{{ d.submitter_telegram_id }}</td>
            <td>{{ formatTime(d.updated_at) }}</td>
            <td><RouterLink :to="`/admin/review/${d.id}`" class="button">查看</RouterLink></td>
          </tr>
        </tbody>
      </table>
    </div>
  </AdminLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import AdminLayout from '@/components/AdminLayout.vue';
import { adminAPI } from '@/api/client';

const drafts = ref<any[]>([]);
const loading = ref(true);

onMounted(async () => {
  try {
    const r = await adminAPI.listDrafts('review');
    drafts.value = r.drafts || [];
  } catch {
    drafts.value = [];
  } finally {
    loading.value = false;
  }
});

function formatTime(value: string): string {
  if (!value) return '';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString('zh-CN');
}
</script>
