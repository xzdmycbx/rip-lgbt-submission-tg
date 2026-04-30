<template>
  <AdminLayout>
    <h2>设置</h2>
    <div class="admin-card">
      <h3>Telegram 机器人</h3>
      <label>
        <span>Bot Token <small v-if="settings.bot_token_set">（已设置: {{ settings.bot_token }}）</small></span>
        <input v-model="form.bot_token" type="password" placeholder="从 @BotFather 获取的 Token" autocomplete="off" />
      </label>
      <label>
        <span>Bot 用户名 (用于生成投稿引导链接)</span>
        <input v-model="form.bot_username" placeholder="rip_lgbt_bot" />
      </label>
      <label>
        <span>模式</span>
        <select v-model="form.bot_mode">
          <option value="polling">long polling</option>
          <option value="webhook">webhook</option>
        </select>
      </label>
      <label v-if="form.bot_mode === 'webhook'">
        <span>Webhook URL</span>
        <input v-model="form.bot_webhook_url" placeholder="https://example.com/api/bot/webhook" />
      </label>
      <label v-if="form.bot_mode === 'webhook'">
        <span>
          Webhook Secret Token
          <small v-if="settings.bot_webhook_secret_set">（已设置: {{ settings.bot_webhook_secret }}）</small>
        </span>
        <input
          v-model="form.bot_webhook_secret"
          type="password"
          autocomplete="off"
          placeholder="A-Z a-z 0-9 _ - 1-256 字符；可选但强烈建议"
        />
      </label>

      <h3 style="margin-top:1.4rem;">站点</h3>
      <label>
        <span>站点显示名</span>
        <input v-model="form.site_name" placeholder="勿忘我" />
      </label>

      <div class="row" style="margin-top:1.2rem;">
        <button class="button primary" type="button" @click="save" :disabled="busy">保存</button>
        <span v-if="status" style="color:var(--muted);">{{ status }}</span>
      </div>
    </div>
  </AdminLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import AdminLayout from '@/components/AdminLayout.vue';
import { adminAPI, type SettingsState } from '@/api/client';

const settings = ref<SettingsState>({});
const status = ref('');
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
    status.value = e?.response?.data?.error || '加载失败';
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
    await adminAPI.updateSettings(patch);
    status.value = '已保存';
    await load();
  } catch (e: any) {
    status.value = e?.response?.data?.message || e?.response?.data?.error || '保存失败';
  } finally {
    busy.value = false;
  }
}

onMounted(load);
</script>
