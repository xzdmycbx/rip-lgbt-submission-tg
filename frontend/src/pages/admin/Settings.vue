<template>
  <AdminLayout>
    <div class="page-head">
      <div>
        <h2 class="page-title">设置</h2>
        <p class="page-subtitle">保存后会自动重载机器人，无需手动重启容器。</p>
      </div>
    </div>

    <form class="card" @submit.prevent="save">
      <h3>Telegram 机器人</h3>
      <p class="card-subtitle">从 <a href="https://t.me/BotFather" target="_blank" rel="noopener">@BotFather</a> 创建并获取 token。</p>

      <div class="field">
        <span class="label">Bot Token <small v-if="settings.bot_token_set" class="field-hint">已设置: {{ settings.bot_token }}</small></span>
        <input v-model="form.bot_token" type="password" autocomplete="off" placeholder="留空表示不修改" />
      </div>

      <div class="field-row">
        <div class="field">
          <span class="label">Bot 用户名</span>
          <input v-model="form.bot_username" placeholder="rip_lgbt_bot" />
        </div>
        <div class="field">
          <span class="label">模式</span>
          <select v-model="form.bot_mode">
            <option value="polling">long polling（推荐）</option>
            <option value="webhook">webhook</option>
          </select>
        </div>
      </div>

      <template v-if="form.bot_mode === 'webhook'">
        <div class="field">
          <span class="label">Webhook URL</span>
          <input v-model="form.bot_webhook_url" placeholder="https://example.com/api/bot/webhook" />
          <p class="field-hint">系统会自动追加 <code>/tg</code> 作为路径，如 <code>https://example.com/api/bot/webhook/tg</code>。</p>
        </div>
        <div class="field">
          <span class="label">
            Webhook Secret Token
            <small v-if="settings.bot_webhook_secret_set" class="field-hint">已设置: {{ settings.bot_webhook_secret }}</small>
          </span>
          <input v-model="form.bot_webhook_secret" type="password" autocomplete="off" placeholder="A-Z a-z 0-9 _ - 1-256 字符" />
        </div>
      </template>

      <h3 class="section-h3">站点</h3>
      <div class="field">
        <span class="label">站点显示名</span>
        <input v-model="form.site_name" placeholder="勿忘我" />
      </div>

      <div class="form-actions">
        <button class="button primary" type="submit" :disabled="busy">保存并重载机器人</button>
        <span v-if="status" class="status-line" :class="statusTone">{{ status }}</span>
      </div>
    </form>
  </AdminLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import AdminLayout from '@/components/AdminLayout.vue';
import { adminAPI, type SettingsState } from '@/api/client';

const settings = ref<SettingsState>({});
const status = ref('');
const statusTone = ref<'ok' | 'error' | ''>('');
const busy = ref(false);

const form = reactive<SettingsState>({
  bot_token: '',
  bot_mode: 'polling',
  bot_webhook_url: '',
  bot_webhook_secret: '',
  bot_username: '',
  site_name: ''
});

async function load() {
  try {
    const r = await adminAPI.getSettings();
    settings.value = r.settings || {};
    form.bot_mode = settings.value.bot_mode || 'polling';
    form.bot_webhook_url = settings.value.bot_webhook_url || '';
    form.bot_username = settings.value.bot_username || '';
    form.site_name = settings.value.site_name || '勿忘我';
    form.bot_token = '';
    form.bot_webhook_secret = '';
  } catch (e: any) {
    setStatus(e?.response?.data?.error || '加载失败', 'error');
  }
}

async function save() {
  busy.value = true;
  status.value = '';
  try {
    const patch: Partial<SettingsState> = {
      bot_mode: form.bot_mode,
      bot_webhook_url: form.bot_webhook_url,
      bot_username: form.bot_username,
      site_name: form.site_name
    };
    if (form.bot_token) patch.bot_token = form.bot_token;
    if (form.bot_webhook_secret) patch.bot_webhook_secret = form.bot_webhook_secret;
    const r: any = await adminAPI.updateSettings(patch);
    if (r.bot_reload_warn) {
      setStatus('已保存。机器人重载警告：' + r.bot_reload_warn, 'error');
    } else if (r.bot_reloaded) {
      setStatus('已保存，机器人已自动重载', 'ok');
    } else {
      setStatus('已保存', 'ok');
    }
    await load();
  } catch (e: any) {
    setStatus(e?.response?.data?.message || e?.response?.data?.error || '保存失败', 'error');
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
