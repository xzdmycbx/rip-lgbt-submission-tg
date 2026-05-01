/**
 * Helpers shared by passkey-related Vue pages: encoding/decoding bytes
 * for the WebAuthn JSON wire format and translating low-level errors
 * into Chinese user-facing messages.
 */

export function base64urlToBuffer(s: string): ArrayBuffer {
  const pad = '='.repeat((4 - (s.length % 4)) % 4);
  const b64 = (s + pad).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(b64);
  const buf = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) buf[i] = raw.charCodeAt(i);
  return buf.buffer;
}

export function bufferToBase64url(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let bin = '';
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export function describePasskeyError(err: any): string {
  if (!err) return 'Passkey 操作失败';
  const msg = (err?.response?.data?.error || err?.message || '').toString();
  if (msg.toLowerCase().includes('notallowed')) return '操作被取消或超时';
  if (msg.toLowerCase().includes('aborted')) return '操作被取消';
  if (msg.toLowerCase().includes('not_pending')) return '请重新发起绑定流程';
  return msg || 'Passkey 操作失败';
}
