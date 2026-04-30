import axios from 'axios';

export const api = axios.create({
  baseURL: '',
  withCredentials: true,
  timeout: 20000
});

api.interceptors.response.use(
  (r) => r,
  (err) => {
    return Promise.reject(err);
  }
);

export interface Person {
  id: string;
  path: string;
  name: string;
  desc: string;
  departure: string;
  profileUrl: string;
  facts: { label: string; value: string }[];
  websites: { label: string; url: string }[];
}

export interface Profile extends Person {
  contentHtml: string;
}

export interface Comment {
  id: string;
  author: string;
  content: string;
  createdAt: string;
}

export interface EngagementSummary {
  flowers: number;
  comments: Comment[];
}

export const memorialsAPI = {
  list: () => api.get<{ count: number; people: Person[] }>('/api/memorials').then((r) => r.data),
  get: (id: string) => api.get<Profile>(`/api/memorials/${encodeURIComponent(id)}`).then((r) => r.data),
  engagement: (id: string) =>
    api.get<EngagementSummary>(`/api/memorials/${encodeURIComponent(id)}/engagement`).then((r) => r.data),
  flower: (id: string) =>
    api
      .post<{ ok: boolean; counted: boolean; flowers: number }>(`/api/memorials/${encodeURIComponent(id)}/flowers`)
      .then((r) => r.data),
  postComment: (id: string, payload: { author: string; content: string; website?: string }) =>
    api
      .post<{ ok: boolean; comment: Comment; summary: EngagementSummary }>(
        `/api/memorials/${encodeURIComponent(id)}/comments`,
        payload
      )
      .then((r) => r.data)
};

export interface AdminInfo {
  id: number;
  username?: string;
  telegram_id?: number;
  display_name: string;
  is_super: boolean;
  has_passkey: boolean;
  totp_confirmed: boolean;
  must_setup_2fa: boolean;
}

export interface LoginNeed {
  totp?: boolean;
  totp_setup?: boolean;
  passkey_setup?: boolean;
  must_setup_2fa?: boolean;
}

export const authAPI = {
  me: () => api.get<{ admin: AdminInfo | null }>('/api/auth/me').then((r) => r.data),
  login: (payload: { username: string; password: string; totp?: string }) =>
    api
      .post<{ ok?: boolean; admin?: AdminInfo; need?: LoginNeed; error?: string }>('/api/auth/login', payload)
      .then((r) => r.data),
  loginTG: (token: string) =>
    api
      .post<{ ok?: boolean; admin?: AdminInfo; need?: LoginNeed; error?: string }>('/api/auth/login/tg', { token })
      .then((r) => r.data),
  logout: () => api.post('/api/auth/logout').then((r) => r.data),
  totpBegin: () =>
    api.post<{ ok: boolean; secret: string; otpauth: string }>('/api/auth/2fa/totp/begin').then((r) => r.data),
  totpConfirm: (code: string) => api.post('/api/auth/2fa/totp/confirm', { code }).then((r) => r.data),
  passkeyRegisterBegin: () => api.post('/api/auth/2fa/passkey/register/begin').then((r) => r.data),
  passkeyRegisterFinish: (body: any) => api.post('/api/auth/2fa/passkey/register/finish', body).then((r) => r.data),
  passkeyDiscoverableBegin: () =>
    api
      .post<{ options: any; challenge_token: string }>('/api/auth/passkey/login/discoverable/begin')
      .then((r) => r.data),
  passkeyDiscoverableFinish: (challenge_token: string, response: any) =>
    api
      .post<{ ok: boolean; admin: AdminInfo }>('/api/auth/passkey/login/discoverable/finish', {
        challenge_token,
        response
      })
      .then((r) => r.data)
};

export interface SettingsState {
  bot_token?: string;
  bot_token_set?: string;
  bot_mode?: string;
  bot_webhook_url?: string;
  bot_webhook_secret?: string;
  bot_webhook_secret_set?: string;
  bot_username?: string;
  site_name?: string;
}

export const adminAPI = {
  listAdmins: () =>
    api.get<{ admins: any[] }>('/api/admin/admins').then((r) => r.data),
  createAdmin: (payload: any) =>
    api.post('/api/admin/admins', payload).then((r) => r.data),
  deleteAdmin: (id: number) =>
    api.delete(`/api/admin/admins/${id}`).then((r) => r.data),
  issueLoginLink: (id: number) =>
    api.post<{ ok: boolean; url: string; expires_in_seconds: number }>(`/api/admin/admins/${id}/login-link`).then((r) => r.data),
  getSettings: () =>
    api.get<{ settings: SettingsState }>('/api/admin/settings').then((r) => r.data),
  updateSettings: (patch: Partial<SettingsState>) =>
    api.put('/api/admin/settings', patch).then((r) => r.data),
  listDrafts: (status = 'review') =>
    api.get<{ drafts: any[] }>(`/api/admin/drafts?status=${encodeURIComponent(status)}`).then((r) => r.data),
  getDraft: (id: string) =>
    api.get<{ draft: any }>(`/api/admin/drafts/${encodeURIComponent(id)}`).then((r) => r.data),
  acceptDraft: (id: string) =>
    api.post(`/api/admin/drafts/${encodeURIComponent(id)}/accept`).then((r) => r.data),
  rejectDraft: (id: string, reason: string) =>
    api.post(`/api/admin/drafts/${encodeURIComponent(id)}/reject`, { reason }).then((r) => r.data),
  requestRevision: (id: string, section: string, note: string) =>
    api.post(`/api/admin/drafts/${encodeURIComponent(id)}/request-revision`, { section, note }).then((r) => r.data)
};
