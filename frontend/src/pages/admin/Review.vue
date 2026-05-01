<template>
  <AdminLayout>
    <div class="page-head">
      <div>
        <h2 class="page-title">审稿 #{{ draft?.entry_id || draftId.slice(0, 8) }}</h2>
        <p class="page-subtitle" v-if="draft">
          提交人 TG <code>{{ draft.submitter_telegram_id }}</code> · 状态
          <span class="badge" :class="statusBadge(draft.status)">{{ draft.status }}</span>
        </p>
      </div>
      <div class="actions">
        <RouterLink class="button ghost" to="/admin/queue">← 返回队列</RouterLink>
      </div>
    </div>

    <div v-if="!draft" class="card empty-state">加载中…</div>
    <div v-else class="review-grid">
      <div class="preview-frame">
        <iframe :src="`/admin/preview/${draftId}`" title="预览"></iframe>
      </div>

      <div class="review-side">
        <div class="card tight">
          <h3>操作</h3>
          <p class="card-subtitle">这位投稿者也会同步收到机器人的通知。</p>
          <div class="action-stack">
            <button class="button primary" type="button" @click="accept" :disabled="busy">
              ✅ 接受 · 立即上线
            </button>
            <button class="button" type="button" @click="rejectDialog = !rejectDialog" :disabled="busy">
              ✗ 拒绝投稿
            </button>
          </div>

          <div v-if="rejectDialog" style="margin-top: 12px;">
            <label class="field">
              <span class="label">拒绝原因（会发给投稿者）</span>
              <textarea v-model="rejectReason" rows="3" placeholder="请说明原因"></textarea>
            </label>
            <div style="display:flex; gap:8px;">
              <button class="button" type="button" @click="rejectDialog=false">取消</button>
              <button class="button danger" type="button" @click="reject" :disabled="busy || !rejectReason.trim()">确认拒绝</button>
            </div>
          </div>
        </div>

        <div class="card tight">
          <h3>退回某一节修改</h3>
          <p class="card-subtitle">机器人会让投稿者重新填写选定的字段。</p>
          <label class="field">
            <span class="label">需要修改的章节</span>
            <select v-model="revisionSection">
              <option value="">— 选择 —</option>
              <option v-for="s in sections" :key="s.value" :value="s.value">{{ s.label }}</option>
            </select>
          </label>
          <label class="field">
            <span class="label">备注（可选）</span>
            <textarea v-model="revisionNote" rows="2" placeholder="例如：希望在简介中补充 ta 的爱好"></textarea>
          </label>
          <button class="button" type="button" @click="requestRevision" :disabled="busy || !revisionSection">退回修改</button>
        </div>

        <div class="card tight">
          <h3>已收集字段</h3>
          <pre class="markdown-dump">{{ JSON.stringify(draft.payload, null, 2) }}</pre>
        </div>

        <div class="card tight" v-if="draft.assets?.length">
          <h3>已上传图片</h3>
          <div class="image-grid">
            <a v-for="a in draft.assets" :key="a.id" :href="`/media/${a.path}`" target="_blank" rel="noopener">
              <img :src="`/media/${a.path}`" :alt="a.role" />
            </a>
          </div>
        </div>

        <div class="card tight">
          <h3>原始 Markdown</h3>
          <pre class="markdown-dump">{{ draft.markdown_full || '(暂无)' }}</pre>
        </div>
      </div>
    </div>

    <p v-if="status" class="status-line" :class="statusTone">{{ status }}</p>
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
const statusTone = ref<'ok' | 'error' | ''>('');
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
    setStatus(e?.response?.data?.error || '加载失败', 'error');
  }
}

async function accept() {
  busy.value = true;
  try {
    await adminAPI.acceptDraft(props.draftId);
    setStatus('已接受 · 跳转中', 'ok');
    setTimeout(() => router.replace('/admin/queue'), 600);
  } catch (e: any) {
    setStatus(e?.response?.data?.error || '操作失败', 'error');
  } finally {
    busy.value = false;
  }
}

async function reject() {
  if (!rejectReason.value.trim()) return;
  busy.value = true;
  try {
    await adminAPI.rejectDraft(props.draftId, rejectReason.value);
    setStatus('已拒绝', 'ok');
    setTimeout(() => router.replace('/admin/queue'), 600);
  } catch (e: any) {
    setStatus(e?.response?.data?.error || '操作失败', 'error');
  } finally {
    busy.value = false;
  }
}

async function requestRevision() {
  if (!revisionSection.value) return;
  busy.value = true;
  try {
    await adminAPI.requestRevision(props.draftId, revisionSection.value, revisionNote.value);
    setStatus('已通知投稿者修改', 'ok');
    setTimeout(() => router.replace('/admin/queue'), 600);
  } catch (e: any) {
    setStatus(e?.response?.data?.error || '操作失败', 'error');
  } finally {
    busy.value = false;
  }
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

function setStatus(text: string, tone: 'ok' | 'error') {
  status.value = text;
  statusTone.value = tone;
}

onMounted(load);
</script>
