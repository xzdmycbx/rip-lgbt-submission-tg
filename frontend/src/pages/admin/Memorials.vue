<template>
  <AdminLayout>
    <div class="page-head">
      <div>
        <h2 class="page-title">已发布稿件</h2>
        <p class="page-subtitle">所有上线的纪念条目；可以编辑或下线。</p>
      </div>
      <div class="actions">
        <input class="search-input" type="search" v-model="q" @input="loadDebounced" placeholder="按 ID 或展示名搜索" />
        <select v-model="status" @change="load">
          <option value="">全部</option>
          <option value="published">已上线</option>
          <option value="archived">已下线</option>
        </select>
      </div>
    </div>

    <div v-if="loading" class="card empty-state">加载中…</div>
    <div v-else-if="!list.length" class="card empty-state">暂无已发布稿件。</div>
    <div v-else class="card tablecard">
      <table class="data">
        <thead>
          <tr><th>ID</th><th>展示名</th><th>逝世日期</th><th>状态</th><th>更新时间</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="m in list" :key="m.id">
            <td><code>{{ m.id }}</code></td>
            <td>{{ m.display_name }}</td>
            <td>{{ m.death_date }}</td>
            <td>
              <span class="badge" :class="m.status === 'published' ? 'ok' : 'muted'">{{ m.status }}</span>
            </td>
            <td>{{ formatTime(m.updated_at) }}</td>
            <td class="row-actions">
              <a class="button sm" :href="`/memorial/${encodeURIComponent(m.id)}`" target="_blank" rel="noopener">查看</a>
              <RouterLink class="button sm" :to="`/admin/memorials/${encodeURIComponent(m.id)}`">编辑</RouterLink>
              <button v-if="m.status === 'published'" class="button danger sm" type="button" @click="archive(m.id)">下线</button>
              <button v-else class="button sm" type="button" @click="restore(m.id)">恢复</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <p v-if="status_msg" class="status-line" :class="statusTone">{{ status_msg }}</p>
  </AdminLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import AdminLayout from '@/components/AdminLayout.vue';
import { adminAPI } from '@/api/client';

const list = ref<any[]>([]);
const loading = ref(true);
const q = ref('');
const status = ref('');
const status_msg = ref('');
const statusTone = ref<'ok' | 'error' | ''>('');

let debounceTimer: any = null;

async function load() {
  loading.value = true;
  try {
    const r = await adminAPI.listMemorials(q.value, status.value);
    list.value = r.memorials || [];
  } catch (e: any) {
    list.value = [];
    setStatus(e?.response?.data?.error || '加载失败', 'error');
  } finally {
    loading.value = false;
  }
}

function loadDebounced() {
  clearTimeout(debounceTimer);
  debounceTimer = setTimeout(load, 220);
}

async function archive(id: string) {
  if (!confirm('下线后将不再显示在公开列表上，但数据保留。继续？')) return;
  try {
    await adminAPI.deleteMemorial(id, false);
    setStatus('已下线', 'ok');
    await load();
  } catch (e: any) {
    setStatus(e?.response?.data?.error || '操作失败', 'error');
  }
}

async function restore(id: string) {
  // restore = update status back to published
  try {
    const cur = await adminAPI.getMemorial(id);
    await adminAPI.updateMemorial(id, { ...cur.memorial, status: 'published' });
    setStatus('已恢复上线', 'ok');
    await load();
  } catch (e: any) {
    setStatus(e?.response?.data?.error || '操作失败', 'error');
  }
}

function formatTime(s: string): string {
  if (!s) return '';
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString('zh-CN');
}

function setStatus(t: string, tone: 'ok' | 'error') {
  status_msg.value = t;
  statusTone.value = tone;
}

onMounted(load);
</script>
