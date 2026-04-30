<template>
  <section id="remembrance" class="profile-section engagement-section" aria-labelledby="remembrance-title">
    <div class="engagement-head">
      <div>
        <p class="eyebrow">REMEMBRANCE</p>
        <h2 id="remembrance-title">献花与留言</h2>
      </div>
      <button
        class="flower-button"
        type="button"
        :aria-label="`为 ${name} 献花`"
        :data-bloom="bloom"
        @click="addFlower"
        :disabled="flowering"
      >
        <span aria-hidden="true">✦</span>
        <span>献花</span>
        <strong>{{ summary?.flowers ?? 0 }}</strong>
      </button>
    </div>

    <div class="comment-shell">
      <form class="comment-form" @submit.prevent="submitComment">
        <label>
          <span>称呼</span>
          <input v-model="form.author" maxlength="40" autocomplete="name" placeholder="访客" />
        </label>
        <label>
          <span>留言</span>
          <textarea v-model="form.content" maxlength="1000" rows="4" required placeholder="写下一句想留下的话"></textarea>
        </label>
        <label class="hp-field" aria-hidden="true">
          <span>Website</span>
          <input v-model="form.website" tabindex="-1" autocomplete="off" />
        </label>
        <div class="comment-actions">
          <button class="button primary" type="submit" :disabled="posting">发送留言</button>
          <p class="comment-status" :data-tone="statusTone" role="status">{{ status }}</p>
        </div>
      </form>

      <div class="comments-area">
        <div class="comments-title">
          <h3>留言</h3>
          <span>{{ summary?.comments?.length ?? 0 }} 条</span>
        </div>
        <ol class="comments-list">
          <li v-for="c in summary?.comments ?? []" :key="c.id" class="comment-item">
            <div class="comment-meta">
              <strong>{{ c.author }}</strong>
              <span>{{ formatTime(c.createdAt) }}</span>
            </div>
            <p class="comment-content">{{ c.content }}</p>
          </li>
          <li v-if="!summary?.comments?.length" class="comment-empty">尚无留言。</li>
        </ol>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { memorialsAPI, type EngagementSummary } from '@/api/client';

const props = defineProps<{ id: string; name: string }>();

const summary = ref<EngagementSummary | null>(null);
const status = ref('');
const statusTone = ref<'' | 'ok' | 'error'>('');
const posting = ref(false);
const flowering = ref(false);
const bloom = ref(false);
const form = reactive({ author: '', content: '', website: '' });

async function refresh() {
  try {
    summary.value = await memorialsAPI.engagement(props.id);
  } catch {
    status.value = '加载失败';
    statusTone.value = 'error';
  }
}

async function addFlower() {
  if (flowering.value) return;
  flowering.value = true;
  try {
    const r = await memorialsAPI.flower(props.id);
    if (summary.value) summary.value.flowers = r.flowers;
    if (r.counted) {
      bloom.value = true;
      setTimeout(() => (bloom.value = false), 600);
    } else {
      status.value = '已经为 ta 献过花了，请明天再来。';
      statusTone.value = 'ok';
    }
  } catch {
    status.value = '献花失败';
    statusTone.value = 'error';
  } finally {
    flowering.value = false;
  }
}

async function submitComment() {
  if (!form.content.trim()) {
    status.value = '请写下想说的话';
    statusTone.value = 'error';
    return;
  }
  posting.value = true;
  status.value = '发送中…';
  statusTone.value = '';
  try {
    const r = await memorialsAPI.postComment(props.id, {
      author: form.author,
      content: form.content,
      website: form.website
    });
    summary.value = r.summary;
    form.content = '';
    status.value = '已发送';
    statusTone.value = 'ok';
  } catch (err: any) {
    const data = err?.response?.data;
    status.value = data?.message || '发送失败';
    statusTone.value = 'error';
  } finally {
    posting.value = false;
  }
}

function formatTime(value: string): string {
  if (!value) return '';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString('zh-CN');
}

onMounted(refresh);
</script>
