<template>
  <AdminLayout>
    <div class="page-head">
      <div>
        <h2 class="page-title">编辑稿件 · {{ id }}</h2>
        <p class="page-subtitle">字段保存后会自动生成 markdown 与 facts，无需手改。</p>
      </div>
      <div class="actions">
        <RouterLink class="button ghost" to="/admin/memorials">← 返回列表</RouterLink>
        <a class="button" :href="`/memorial/${encodeURIComponent(id)}`" target="_blank" rel="noopener">在新标签查看</a>
      </div>
    </div>

    <div v-if="!memorial" class="card empty-state">加载中…</div>
    <form v-else class="card" @submit.prevent="save">
      <h3>基础信息</h3>
      <div class="field-row">
        <div class="field"><span class="label">展示名</span><input type="text" v-model="memorial.display_name" required /></div>
        <div class="field"><span class="label">头像 URL</span><input type="text" v-model="memorial.avatar_url" placeholder="/media/memorials/xxx.jpg" /></div>
        <div class="field"><span class="label">一句话简介</span><input type="text" v-model="memorial.description" /></div>
        <div class="field"><span class="label">地区</span><input type="text" v-model="memorial.location" /></div>
        <div class="field"><span class="label">出生日期</span><input type="text" v-model="memorial.birth_date" /></div>
        <div class="field"><span class="label">逝世日期</span><input type="text" v-model="memorial.death_date" /></div>
        <div class="field"><span class="label">昵称</span><input type="text" v-model="memorial.alias" /></div>
        <div class="field"><span class="label">年龄</span><input type="text" v-model="memorial.age" /></div>
        <div class="field"><span class="label">身份表述</span><input type="text" v-model="memorial.identity" /></div>
        <div class="field"><span class="label">代词</span><input type="text" v-model="memorial.pronouns" /></div>
        <div class="field">
          <span class="label">状态</span>
          <select v-model="memorial.status">
            <option value="published">已上线</option>
            <option value="archived">已下线</option>
          </select>
        </div>
      </div>

      <h3 class="section-h3">正文章节</h3>
      <div class="field"><span class="label">简介</span><textarea v-model="memorial.intro" rows="4"></textarea></div>
      <div class="field"><span class="label">生平与记忆</span><textarea v-model="memorial.life" rows="6"></textarea></div>
      <div class="field"><span class="label">离世</span><textarea v-model="memorial.death" rows="4"></textarea></div>
      <div class="field"><span class="label">念想</span><textarea v-model="memorial.remembrance" rows="6"></textarea></div>

      <h3 class="section-h3">附加内容</h3>
      <div class="field"><span class="label">公开链接（每行一个，如 “twitter: https://...”）</span><textarea v-model="memorial.links_md" rows="3"></textarea></div>
      <div class="field"><span class="label">作品（每行一项）</span><textarea v-model="memorial.works_md" rows="3"></textarea></div>
      <div class="field"><span class="label">资料来源</span><textarea v-model="memorial.sources_md" rows="2"></textarea></div>
      <div class="field"><span class="label">自选附加项</span><textarea v-model="memorial.custom_md" rows="3"></textarea></div>

      <div class="form-actions">
        <button class="button primary" type="submit" :disabled="busy">保存</button>
        <button class="button danger" type="button" @click="hardDelete" :disabled="busy">永久删除</button>
        <span v-if="status" class="status-line" :class="statusTone">{{ status }}</span>
      </div>
    </form>

    <details class="card" v-if="memorial">
      <summary class="card-summary">查看自动生成的 Markdown 与 Facts JSON</summary>
      <p class="card-subtitle section-h3">这两项由后端从结构化字段自动生成，仅供查看。下次保存会自动更新。</p>
      <h4 class="subtitle-mini">MARKDOWN_FULL</h4>
      <pre class="markdown-dump">{{ memorial.markdown_full }}</pre>
      <h4 class="subtitle-mini">FACTS_JSON</h4>
      <pre class="markdown-dump">{{ JSON.stringify(memorial.facts, null, 2) }}</pre>
      <h4 class="subtitle-mini">WEBSITES_JSON</h4>
      <pre class="markdown-dump">{{ JSON.stringify(memorial.websites, null, 2) }}</pre>
    </details>

    <div v-if="memorial" class="card">
      <h3>预览</h3>
      <p class="card-subtitle">和公开页同款渲染。</p>
      <div class="story" v-html="contentHtml"></div>
    </div>
  </AdminLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import AdminLayout from '@/components/AdminLayout.vue';
import { adminAPI } from '@/api/client';

const props = defineProps<{ id: string }>();
const router = useRouter();

const memorial = ref<any | null>(null);
const contentHtml = ref('');
const status = ref('');
const statusTone = ref<'ok' | 'error' | ''>('');
const busy = ref(false);

async function load() {
  try {
    const r = await adminAPI.getMemorial(props.id);
    memorial.value = r.memorial;
    contentHtml.value = r.content_html;
  } catch (e: any) {
    setStatus(e?.response?.data?.error || '加载失败', 'error');
  }
}

async function save() {
  busy.value = true;
  status.value = '';
  try {
    const r: any = await adminAPI.updateMemorial(props.id, memorial.value);
    if (r?.markdown_full) memorial.value.markdown_full = r.markdown_full;
    if (r?.facts) memorial.value.facts = r.facts;
    if (r?.websites) memorial.value.websites = r.websites;
    setStatus('已保存，markdown 与 facts 已自动同步', 'ok');
    await load();
  } catch (e: any) {
    setStatus(e?.response?.data?.error || '保存失败', 'error');
  } finally {
    busy.value = false;
  }
}

async function hardDelete() {
  if (!confirm('永久删除这条纪念条目？此操作不可撤销，相关献花和留言也会一并删除。')) return;
  busy.value = true;
  try {
    await adminAPI.deleteMemorial(props.id, true);
    router.replace('/admin/memorials');
  } catch (e: any) {
    setStatus(e?.response?.data?.error || '删除失败', 'error');
  } finally {
    busy.value = false;
  }
}

function setStatus(t: string, tone: 'ok' | 'error') {
  status.value = t;
  statusTone.value = tone;
}

onMounted(load);
</script>
