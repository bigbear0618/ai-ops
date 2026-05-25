import { request } from './client';

// IM bridge app CRUD — admin endpoints for managing Feishu / DingTalk /
// Telegram bot registrations. Default mode is `stream` so manager dials
// out; webhook is fallback for inbound-only environments. Telegram is
// stream-only (getUpdates long-poll) and REQUIRES allow_from — a sender
// allowlist — because the bot is publicly reachable (ADR-031).

export type IMProvider = 'feishu' | 'dingtalk' | 'telegram';
export type IMMode = 'stream' | 'webhook';

export type IMApp = {
  id: number;
  provider: IMProvider;
  mode: IMMode;
  name: string;
  app_id: string;
  has_secret: boolean;
  verify_token?: string;
  encrypt_key?: string;
  // Telegram sender allowlist: comma-separated numeric Telegram user IDs.
  allow_from?: string;
  enabled: boolean;
  idle_timeout_seconds: number;
  created_at: string;
  updated_at: string;
};

export type IMAppPayload = {
  provider: IMProvider;
  mode: IMMode;
  name: string;
  app_id: string;
  // Empty on update = keep current secret (per backend contract).
  app_secret?: string;
  verify_token?: string;
  encrypt_key?: string;
  allow_from?: string;
  enabled: boolean;
};

export type IMAppListResp = {
  items: IMApp[];
  total: number;
};

export function listIMApps(provider?: IMProvider): Promise<IMAppListResp> {
  const qs = provider ? `?provider=${encodeURIComponent(provider)}` : '';
  return request<IMAppListResp>('GET', `/im/apps${qs}`);
}

export function getIMApp(id: number): Promise<IMApp> {
  return request<IMApp>('GET', `/im/apps/${id}`);
}

export function createIMApp(payload: IMAppPayload): Promise<IMApp> {
  return request<IMApp>('POST', '/im/apps', payload);
}

export function updateIMApp(id: number, payload: IMAppPayload): Promise<IMApp> {
  return request<IMApp>('PUT', `/im/apps/${id}`, payload);
}

export function deleteIMApp(id: number): Promise<void> {
  return request<void>('DELETE', `/im/apps/${id}`);
}

export function revealIMAppSecret(id: number): Promise<{ app_secret: string }> {
  return request<{ app_secret: string }>('POST', `/im/apps/${id}/reveal`, {});
}
