import { createApp } from 'vue';
import { createPinia } from 'pinia';
import App from './App.vue';
import { router } from './router';
import { useThemeStore } from './stores/theme';
import './styles/global.css';

const app = createApp(App);
app.use(createPinia());
app.use(router);
// Apply persisted theme before mounting so the initial paint already
// matches the user's preference and we avoid a dark/light flash.
useThemeStore().apply();
app.mount('#app');
