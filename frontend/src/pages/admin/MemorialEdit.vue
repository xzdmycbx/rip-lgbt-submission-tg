<template>
  <AdminLayout>
    <div class="page-head">
      <div>
        <h2 class="page-title">编辑稿件 · {{ id }}</h2>
        <p class="page-subtitle">字段直接对应数据库列；保存即立刻在公开页生效。</p>
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
        <div class="field"><span class="label">展示名</span><input v-model="memorial.display_name" required /></div>
        <div class="field"><span class="label">头像 URL</span><input v-model="memorial.avatar_url" placeholder="/media/memorials/xxx.jpg" /></div>
        <div class="field"><span class="label">一句话简介</span><input v-model="memorial.description" /></div>
        <div class="field"><span class="label">地区</span><input v-model="memorial.location" /></div>
        <div class="field"><span class="label">出生日期</span><input v-model="memorial.birth_date" /></div>
        <div class="field"><span class="label">逝世日期</span><input v-model="memorial.death_date" /></div>
        <div class="field"><span class="label">昵称</span><input v-model="memorial.alias" /></div>
        <div class="field"><span class="label">年龄</span><input v-model="memorial.age" /></div>
        <div class="field"><span class="label">身份表述</span><input v-model="memorial.identity" /></div>
        <div class="field"><span class="label">代词</span><input v-model="memorial.pronouns" /></div>
        <div class="field">
          <span class="label">状态</span>
          <select v-model="memorial.status">
            <option value="published">已上线</option>
            <option value="archived">已下线</option>
          </select>
        </div>
      </div>

      <h3 style="margin-top: 18px;">正文章节</h3>
      <div class="field"><span class="label">简介</span><textarea v-model="memorial.intro" rows="4"></textarea></div>
      <div class="field"><span class="label">生平与记忆</span><textarea v-model="memorial.life" rows="6"></textarea></div>
      <div class="field"><span class="label">离世</span><textarea v-model="memorial.death" rows="4"></textarea></div>
      <div class="field"><span class="label">念想</span><textarea v-model="memorial.remembrance" rows="6"></textarea></div>

      <h3 style="margin-top: 18px;">附加内容</h3>
      <div class="field"><span class="label">公开链接（每行一个，如 “twitter: https://...”）</span><textarea v-model="memorial.links_md" rows="3"></textarea></div>
      <div class="field"><span class="label">作品（每行一项）</span><textarea v-model="memorial.works_md" rows="3"></textarea></div>
      <div class="field"><span class="label">资料来源</span><textarea v-model="memorial.sources_md" rows="2"></textarea></div>
      <div class="field"><span class="label">自选附加项</span><textarea v-model="memorial.custom_md" rows="3"></textarea></div>

      <h3 style="margin-top: 18px;">完整 Markdown</h3>
      <p class="field-hint">公开页直接渲染这段。如果你单独修改了上面字段，记得同步更新这里。</p>
      <div class="field"><textarea v-model="memorial.markdown_full" rows="14" style="font-family: ui-monospace, SFMono-Regular, Consolas, monospace;"></textarea></div>

      <div style="margin-top: 18px; display: flex; gap: 8px; align-items: center; flex-wrap: wrap;">
        <button class="button primary" type="submit" :disabled="busy">保存</button>
        <button class="button danger" type="button" @click="hardDelete" :disabled="busy">永久删除</button>
        <span v-if="status" class="status-line" :class="statusTone">{{ status }}</span>
      </div>
    </form>

    <div v-if="memorial" class="card">
      <h3>预览</h3>
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
    await adminAPI.updateMemorial(props.id, memorial.value);
    setStatus('已保存', 'ok');
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
