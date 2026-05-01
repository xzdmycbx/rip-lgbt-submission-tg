<template>
  <div class="upload-page">
    <header class="upload-header">
      <h1>{{ draft?.display_name || '投稿图片上传' }}</h1>
      <p class="lede" v-if="draft">
        条目 ID <code>{{ draft.entry_id || '尚未填写' }}</code>。请把头像和正文中要用的图片在这里上传，上传完直接回到 Telegram 机器人继续投稿。
      </p>
      <p v-if="!loaded" class="lede">加载中…</p>
      <p v-if="error" class="error">{{ error }}</p>
      <div class="head-actions" v-if="loaded">
        <button class="btn ghost" type="button" @click="refresh" :disabled="reloading">
          {{ reloading ? '刷新中…' : '🔄 刷新预览' }}
        </button>
        <span class="head-hint">本页内容不会自动同步；改完内容后点刷新查看预览。</span>
      </div>
    </header>

    <section v-for="cat in categories" :key="cat.role" class="upload-cat">
      <div class="upload-cat-head">
        <h2>{{ cat.title }}</h2>
        <span class="badge">{{ assetsByRole(cat.role).length }} {{ cat.multiple ? '张' : '/ 1 张' }}</span>
      </div>

      <label
        class="dropzone"
        :class="{ disabled: !cat.multiple && assetsByRole(cat.role).length >= 1 }"
        @dragover.prevent
        @drop.prevent="(e) => onDrop(e, cat)"
      >
        <input
          class="dz-input"
          type="file"
          accept="image/*"
          :multiple="cat.multiple"
          :disabled="!cat.multiple && assetsByRole(cat.role).length >= 1"
          @change="(e) => onPick(e, cat)"
        />
        <span class="dz-hint">
          点击或拖拽图片到这里
          <span v-if="!cat.multiple">（仅 1 张，再次上传会替换）</span>
          <span v-else>（可多选）</span>
        </span>
        <span class="dz-progress" v-if="uploading[cat.role]">上传中… {{ uploading[cat.role] }}</span>
      </label>

      <div v-if="assetsByRole(cat.role).length" class="thumb-grid">
        <figure v-for="a in assetsByRole(cat.role)" :key="a.id" class="thumb">
          <img :src="a.url" :alt="a.filename" loading="lazy" />
          <figcaption>
            <span class="filename">{{ a.filename }}</span>
            <button class="remove" type="button" @click="remove(a)">删除</button>
          </figcaption>
        </figure>
      </div>
    </section>

    <section v-if="profile" class="preview-card">
      <div class="preview-head">
        <h2>投稿内容预览</h2>
        <span class="head-hint">展示当前已经在机器人里填好的内容；改完后刷新本页同步。</span>
      </div>

      <article class="preview-profile">
        <header class="preview-hero">
          <div class="preview-photo">
            <img v-if="profile.profileUrl" :src="profile.profileUrl" :alt="profile.name" />
            <div v-else class="preview-mark" aria-hidden="true">{{ firstGlyph(profile.name) }}</div>
          </div>
          <div>
            <p class="preview-eyebrow">PREVIEW</p>
            <h3>{{ profile.name || '(尚未填写展示名)' }}</h3>
            <p class="preview-date">{{ formatDate(profile.departure) }}</p>
            <p class="preview-desc">{{ profile.desc || '尚未填写一句话简介。' }}</p>
          </div>
        </header>

        <section v-if="profile.facts.length" class="preview-facts">
          <h4>公开信息</h4>
          <dl>
            <template v-for="f in profile.facts" :key="f.label">
              <div>
                <dt>{{ f.label }}</dt>
                <dd>{{ f.value }}</dd>
              </div>
            </template>
          </dl>
        </section>

        <section v-if="profile.contentHtml" class="preview-body">
          <h4>正文</h4>
          <div class="story" v-html="profile.contentHtml"></div>
        </section>
        <p v-else class="preview-empty">尚未填写正文章节，机器人里继续完成简介 / 生平 / 离世 / 念想后再回来刷新即可看到效果。</p>
      </article>
    </section>

    <p class="page-hint">
      上传完成后请回到 Telegram 机器人继续投稿；如需关掉本页直接关闭即可，已上传的图片会保留。链接默认在投稿提交前一直可用。
    </p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { uploadAPI, type UploadAsset, type UploadCategory } from '@/api/client';
import '@/styles/upload.css';

const props = defineProps<{ token: string }>();

const loaded = ref(false);
const reloading = ref(false);
const error = ref('');
const draft = ref<{ id: string; display_name: string; description: string; entry_id: string; current_step: string; status: string } | null>(null);
const profile = ref<{ name: string; desc: string; departure: string; profileUrl: string; facts: { label: string; value: string }[]; contentHtml: string } | null>(null);
const categories = ref<UploadCategory[]>([]);
const assets = ref<UploadAsset[]>([]);
const uploading = reactive<Record<string, string>>({});
function assetsByRole(role: string): UploadAsset[] {
  return assets.value.filter((a) => a.role === role);
}

async function refresh() {
  reloading.value = true;
  try {
    const r = await uploadAPI.state(props.token);
    draft.value = r.draft;
    profile.value = r.profile;
    categories.value = r.categories || [];
    assets.value = r.assets || [];
    loaded.value = true;
    error.value = '';
  } catch (e: any) {
    error.value = e?.response?.data?.error === 'token_invalid'
      ? '上传链接已失效，请回到 Telegram 机器人重新生成。'
      : e?.response?.data?.error || '加载失败';
  } finally {
    reloading.value = false;
  }
}

async function uploadFiles(cat: UploadCategory, files: FileList | File[]) {
  const arr = Array.from(files);
  for (let i = 0; i < arr.length; i++) {
    if (!cat.multiple && i > 0) break;
    const f = arr[i]!;
    if (!f.type.startsWith('image/')) continue;
    uploading[cat.role] = `${i + 1} / ${arr.length}`;
    try {
      await uploadAPI.upload(props.token, cat.role, f);
    } catch (e: any) {
      const code = e?.response?.data?.error || '';
      error.value = code === 'token_invalid'
        ? '上传链接已失效，请回到 Telegram 机器人重新生成。'
        : `上传 ${f.name} 失败：${code || '未知错误'}`;
      break;
    }
  }
  uploading[cat.role] = '';
  await refresh();
}

async function onDrop(e: DragEvent, cat: UploadCategory) {
  if (!cat.multiple && assetsByRole(cat.role).length >= 1) {
    // For single-slot categories, drop replaces.
  }
  const files = e.dataTransfer?.files;
  if (files && files.length) {
    await uploadFiles(cat, files);
  }
}

async function onPick(e: Event, cat: UploadCategory) {
  const input = e.target as HTMLInputElement;
  if (input.files && input.files.length) {
    await uploadFiles(cat, input.files);
    input.value = '';
  }
}

async function remove(a: UploadAsset) {
  if (!confirm(`删除 ${a.filename}？`)) return;
  try {
    await uploadAPI.remove(props.token, a.id);
    await refresh();
  } catch (e: any) {
    error.value = e?.response?.data?.error || '删除失败';
  }
}

function firstGlyph(name: string): string {
  return Array.from(String(name || '勿').trim())[0] || '勿';
}

function formatDate(value: string): string {
  if (!value) return '日期待考';
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return value;
  const [y, m, d] = value.split('-');
  return `${y}.${m}.${d}`;
}

onMounted(refresh);
</script>
