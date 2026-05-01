<template>
  <AdminLayout>
    <div class="page-head">
      <div>
        <h2 class="page-title">待审投稿</h2>
        <p class="page-subtitle">通过 Telegram 机器人收到的投稿等待维护者审核。</p>
      </div>
      <div class="actions">
        <select v-model="status" @change="load">
          <option value="review">待审 (review)</option>
          <option value="collecting">收集中 (collecting)</option>
          <option value="revising">退回修改 (revising)</option>
          <option value="rejected">已拒绝 (rejected)</option>
          <option value="accepted">已接受 (accepted)</option>
        </select>
        <button class="button" type="button" @click="load" :disabled="loading">刷新</button>
      </div>
    </div>

    <div v-if="loading" class="card empty-state">加载中…</div>
    <div v-else-if="!drafts.length" class="card empty-state">
      <p>当前没有需要审核的投稿。</p>
      <p class="field-hint">用户在机器人里点击「✅ 提交审核」后会出现在这里。</p>
    </div>

    <div v-else>
      <RouterLink
        v-for="d in drafts"
        :key="d.id"
        class="draft-card"
        :to="`/admin/review/${d.id}`"
      >
        <div class="meta">
          <span class="name">{{ d.display_name || '(未填写展示名)' }}</span>
          <span class="submeta">
            <code>{{ d.entry_id || '—' }}</code>
            · 提交人 TG <code>{{ d.submitter_telegram_id }}</code>
            · {{ formatTime(d.updated_at) }}
          </span>
        </div>
        <span class="badge" :class="statusBadge(d.status)">{{ d.status }}</span>
      </RouterLink>
    </div>
  </AdminLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import AdminLayout from '@/components/AdminLayout.vue';
import { adminAPI } from '@/api/client';

const drafts = ref<any[]>([]);
const loading = ref(true);
const status = ref('review');

async function load() {
  loading.value = true;
  try {
    const r = await adminAPI.listDrafts(status.value);
    drafts.value = r.drafts || [];
  } catch {
    drafts.value = [];
  } finally {
    loading.value = false;
  }
}

onMounted(load);

function formatTime(s: string): string {
  if (!s) return '';
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString('zh-CN');
}

function statusBadge(s: string): string {
  switch (s) {
    case 'review': return 'info';
    case 'accepted': return 'ok';
    case 'rejected': return 'danger';
    case 'revising': return 'warn';
    default: return 'muted';
  }
}
</script>
