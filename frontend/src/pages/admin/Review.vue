<template>
  <AdminLayout>
    <h2>审稿 #{{ draftId }}</h2>
    <div class="admin-card" v-if="!draft">加载中…</div>
    <div v-else>
      <div class="review-grid">
        <iframe :src="`/admin/preview/${draftId}`" title="预览"></iframe>
        <div>
          <div class="admin-card">
            <h3>操作</h3>
            <div class="row">
              <button class="button primary" @click="accept" :disabled="busy">接受 · 上线</button>
              <button class="button" @click="rejectDialog = true" :disabled="busy">拒绝</button>
            </div>
            <div style="margin-top:1rem;">
              <h4 style="font-size:.95rem; margin: .4rem 0;">要求修改某一节</h4>
              <select v-model="revisionSection">
                <option value="">选择节</option>
                <option v-for="s in sections" :key="s.value" :value="s.value">{{ s.label }}</option>
              </select>
              <textarea v-model="revisionNote" rows="2" placeholder="给投稿人的话（可选）" style="margin-top:.4rem;"></textarea>
              <button class="button" type="button" @click="requestRevision" :disabled="busy || !revisionSection">退回修改</button>
            </div>
          </div>

          <div class="admin-card">
            <h3>原始 Markdown</h3>
            <pre>{{ draft.markdown_full || '(草稿尚未生成 markdown)' }}</pre>
          </div>

          <div class="admin-card">
            <h3>已收集字段</h3>
            <pre>{{ JSON.stringify(draft.payload, null, 2) }}</pre>
          </div>
        </div>
      </div>

      <div v-if="rejectDialog" class="admin-card">
        <h3>填写拒绝原因</h3>
        <textarea v-model="rejectReason" rows="3" placeholder="将告知投稿人"></textarea>
        <div class="row" style="margin-top:.6rem;">
          <button class="button" @click="rejectDialog = false">取消</button>
          <button class="button primary" @click="reject" :disabled="busy || !rejectReason">确认拒绝</button>
        </div>
      </div>

      <p v-if="status" style="color:var(--muted); margin-top:1rem;">{{ status }}</p>
    </div>
  </AdminLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import AdminLayout from '@/components/AdminLayout.vue';
import { adminAPI } from '@/api/client';

const props = defineProps<{ draftId: string }>();
const router = useRouter();

const draft = ref<any | null>(null);
const status = ref('');
const busy = ref(false);
const rejectDialog = ref(false);
const rejectReason = ref('');
const revisionSection = ref('');
const revisionNote = ref('');

const sections = [
  { value: 'entry_id', label: '条目 ID' },
  { value: 'display_name', label: '展示名' },
  { value: 'avatar', label: '头像' },
  { value: 'description', label: '一句话简介' },
  { value: 'location', label: '地区' },
  { value: 'birth_date', label: '出生日期' },
  { value: 'death_date', label: '逝世日期' },
  { value: 'intro', label: '简介' },
  { value: 'life', label: '生平与记忆' },
  { value: 'death', label: '离世' },
  { value: 'remembrance', label: '念想' },
  { value: 'links', label: '公开链接' },
  { value: 'works', label: '作品' },
  { value: 'sources', label: '资料来源' },
  { value: 'custom', label: '自选附加项' }
];

async function load() {
  try {
    const r = await adminAPI.getDraft(props.draftId);
    draft.value = r.draft;
  } catch (e: any) {
    status.value = e?.response?.data?.error || '加载失败';
  }
}

async function accept() {
  busy.value = true;
  try {
    await adminAPI.acceptDraft(props.draftId);
    status.value = '已接受';
    setTimeout(() => router.replace('/admin/queue'), 800);
  } catch (e: any) {
    status.value = e?.response?.data?.error || '操作失败';
  } finally {
    busy.value = false;
  }
}

async function reject() {
  if (!rejectReason.value) return;
  busy.value = true;
  try {
    await adminAPI.rejectDraft(props.draftId, rejectReason.value);
    status.value = '已拒绝';
    setTimeout(() => router.replace('/admin/queue'), 800);
  } catch (e: any) {
    status.value = e?.response?.data?.error || '操作失败';
  } finally {
    busy.value = false;
  }
}

async function requestRevision() {
  if (!revisionSection.value) return;
  busy.value = true;
  try {
    await adminAPI.requestRevision(props.draftId, revisionSection.value, revisionNote.value);
    status.value = '已退回，机器人会请投稿人补改。';
    setTimeout(() => router.replace('/admin/queue'), 800);
  } catch (e: any) {
    status.value = e?.response?.data?.error || '操作失败';
  } finally {
    busy.value = false;
  }
}

onMounted(load);
</script>
