// IM apps admin. CRUD for Feishu / DingTalk bot
// registrations. Each row drives one long-connection (stream mode)
// or one webhook endpoint that the platform calls. List view is
// table-shaped (consistent with the Users / Edges page); the
// create / edit form is a Modal.

import { useCallback, useEffect, useState } from 'react';
import { Plus, RefreshCw, Loader2, Pencil, Trash2, Eye, EyeOff, MessagesSquare, MessageSquareShare, Send } from 'lucide-react';
import { ApiError } from '@/api/client';
import {
  createIMApp,
  deleteIMApp,
  listIMApps,
  revealIMAppSecret,
  updateIMApp,
  type IMApp,
  type IMAppPayload,
  type IMMode,
  type IMProvider,
} from '@/api/imbridge';
import { Button, Card, Chip, EmptyState } from '@/components/ui';
import { Modal } from '@/components/Modal';
import { cn } from '@/lib/cn';
import { useI18n } from '@/i18n/locale';

const PROVIDER_META: Record<IMProvider, { labelZh: string; labelEn: string; icon: typeof MessageSquareShare; hintZh: string; hintEn: string }> = {
  feishu: {
    labelZh: '飞书',
    labelEn: 'Feishu',
    icon: MessageSquareShare,
    hintZh: '飞书开放平台应用。建议走 stream 模式（长连接）— manager 主动 dial 出去，无需公网回调。',
    hintEn: 'Feishu Open Platform app. Stream mode is the default — manager dials out, no public webhook URL required.',
  },
  dingtalk: {
    labelZh: '钉钉',
    labelEn: 'DingTalk',
    icon: Send,
    hintZh: '钉钉企业内部应用（落地，stream 实现 in progress）。',
    hintEn: 'DingTalk enterprise app (in progress — stream impl pending).',
  },
  telegram: {
    labelZh: 'Telegram',
    labelEn: 'Telegram',
    icon: Send,
    hintZh: 'Telegram bot：app_id 填 bot 用户名，app_secret 填 BotFather 的 token。仅 stream 模式（getUpdates 长轮询，出站走代理）。⚠ bot 公开可搜，必须填 allow_from 白名单，否则任何人都能直接和 agent 对话。',
    hintEn: 'Telegram bot: app_id = bot username, app_secret = the BotFather token. Stream-only (getUpdates long-poll, outbound via proxy). ⚠ the bot is publicly searchable — allow_from is REQUIRED, otherwise anyone could talk to the agent.',
  },
};

export default function SettingsIMApps() {
  const { tr } = useI18n();
  const [items, setItems] = useState<IMApp[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [editing, setEditing] = useState<IMApp | 'create' | null>(null);
  const [deleting, setDeleting] = useState<IMApp | null>(null);

  const fetchAll = useCallback(async (silent = false) => {
    if (silent) setRefreshing(true);
    else setLoading(true);
    try {
      const r = await listIMApps();
      setItems(r.items ?? []);
      setErr(null);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : (e as Error).message);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void fetchAll();
  }, [fetchAll]);

  const onSaved = async () => {
    setEditing(null);
    await fetchAll(true);
  };

  return (
    <>
      {/* Same shell as /settings/llm and /settings/communications:
          intro panel up top describing what this page does, then a
          right-aligned toolbar + table (or EmptyState) below. The
          surrounding SettingsLayout already provides the page-level
          header — we render content only. */}
      <div className="space-y-4">
        <div className="rounded-lg border border-zinc-800/60 bg-zinc-900/30 px-4 py-3 text-[12px] text-zinc-400">
          <div className="mb-1 flex items-center gap-2 text-zinc-200">
            <MessagesSquare size={14} className="text-zinc-400" />
            <span className="font-medium">{tr('通信 — IM 双向 bot', 'Communications — IM bots (two-way)')}</span>
          </div>
          {tr(
            '配置飞书 / 钉钉 / Telegram 机器人，群里 @bot 或私聊就能开多轮会话。推荐 ',
            'Configure Feishu / DingTalk / Telegram bots so users can @bot in a group (or DM) and get multi-turn conversations. ',
          )}
          <b>{tr('stream 模式', 'Stream mode')}</b>
          {tr(
            '：manager 主动拨长连接出去，无需公网回调 URL。改完保存后 ~30 秒内 supervisor 自动重连。',
            ' is recommended — manager dials out via long connection, no public webhook URL required. Supervisor auto-reconnects within ~30 s of saving.',
          )}
        </div>

        {err && (
          <div className="rounded-lg border border-red-500/40 bg-red-500/5 px-3 py-2 text-xs text-red-300">
            {err}
          </div>
        )}

        <div className="flex flex-wrap items-center justify-end gap-2">
          <Button onClick={() => fetchAll(true)} disabled={refreshing || loading} variant="ghost">
            <RefreshCw size={12} className={cn(refreshing && 'animate-spin')} />
            {tr('刷新', 'Refresh')}
          </Button>
          <Button variant="primary" onClick={() => setEditing('create')}>
            <Plus size={12} /> {tr('新建', 'New')}
          </Button>
        </div>

        {loading ? (
          <Card className="p-5">
            <div className="flex h-32 items-center justify-center text-sm text-zinc-500">
              <Loader2 size={14} className="mr-2 animate-spin" /> {tr('加载中…', 'Loading…')}
            </div>
          </Card>
        ) : items.length === 0 ? (
          <EmptyState
            title={tr('还没有 IM bot', 'No IM bots yet')}
            hint={tr(
              '点上面"新建"配第一个机器人。stream 模式无需公网回调，最快上手。',
              'Click "New" above to configure your first bot. Stream mode requires no public webhook URL.',
            )}
          />
        ) : (
          <Card className="overflow-hidden p-0">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-zinc-800/60 text-left text-[11px] uppercase tracking-wide text-zinc-500">
                  <th className="px-4 py-2.5 font-medium">{tr('平台', 'Provider')}</th>
                  <th className="px-4 py-2.5 font-medium">{tr('名称', 'Name')}</th>
                  <th className="px-4 py-2.5 font-medium">app_id</th>
                  <th className="px-4 py-2.5 font-medium">{tr('模式', 'Mode')}</th>
                  <th className="px-4 py-2.5 font-medium">{tr('状态', 'Status')}</th>
                  <th className="px-4 py-2.5 font-medium">{tr('操作', 'Actions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-800/40">
                {items.map((a) => {
                  const meta = PROVIDER_META[a.provider];
                  const Icon = meta?.icon ?? MessageSquareShare;
                  return (
                    <tr key={a.id} className="hover:bg-zinc-900/40">
                      <td className="whitespace-nowrap px-4 py-2.5">
                        <div className="flex items-center gap-2">
                          <span className="rounded-md bg-slate-100 p-1 text-slate-600 dark:bg-zinc-800/70 dark:text-zinc-300">
                            <Icon size={12} />
                          </span>
                          <span className="text-zinc-100">{tr(meta?.labelZh ?? a.provider, meta?.labelEn ?? a.provider)}</span>
                        </div>
                      </td>
                      <td className="whitespace-nowrap px-4 py-2.5 text-zinc-100">{a.name}</td>
                      <td className="whitespace-nowrap px-4 py-2.5 font-mono text-[12px] text-zinc-300">{a.app_id}</td>
                      <td className="whitespace-nowrap px-4 py-2.5">
                        <Chip tone={a.mode === 'stream' ? 'success' : 'warning'}>{a.mode}</Chip>
                      </td>
                      <td className="whitespace-nowrap px-4 py-2.5">
                        <Chip tone={a.enabled ? 'success' : 'default'}>
                          {a.enabled ? tr('已启用', 'Enabled') : tr('已停用', 'Disabled')}
                        </Chip>
                        {!a.has_secret && (
                          <Chip tone="warning" className="ml-1">
                            {tr('缺凭证', 'No secret')}
                          </Chip>
                        )}
                      </td>
                      <td className="whitespace-nowrap px-4 py-2.5">
                        <div className="flex items-center gap-1">
                          <Button onClick={() => setEditing(a)} title={tr('编辑', 'Edit')}>
                            <Pencil size={11} /> {tr('编辑', 'Edit')}
                          </Button>
                          <Button onClick={() => setDeleting(a)} variant="danger" title={tr('删除', 'Delete')}>
                            <Trash2 size={11} /> {tr('删除', 'Delete')}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </Card>
        )}
      </div>

      {editing && (
        <IMAppEditor
          target={editing === 'create' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={onSaved}
        />
      )}
      {deleting && (
        <DeleteConfirm
          target={deleting}
          onClose={() => setDeleting(null)}
          onDeleted={async () => {
            setDeleting(null);
            await fetchAll(true);
          }}
        />
      )}
    </>
  );
}

function IMAppEditor({
  target,
  onClose,
  onSaved,
}: {
  target: IMApp | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { tr } = useI18n();
  const isCreate = target === null;

  const [provider, setProvider] = useState<IMProvider>(target?.provider ?? 'feishu');
  const [mode, setMode] = useState<IMMode>(target?.mode ?? 'stream');
  const [name, setName] = useState(target?.name ?? '');
  const [appID, setAppID] = useState(target?.app_id ?? '');
  const [appSecret, setAppSecret] = useState('');
  const [verifyToken, setVerifyToken] = useState(target?.verify_token ?? '');
  const [encryptKey, setEncryptKey] = useState(target?.encrypt_key ?? '');
  const [allowFrom, setAllowFrom] = useState(target?.allow_from ?? '');
  const [enabled, setEnabled] = useState(target?.enabled ?? true);
  const [revealedSecret, setRevealedSecret] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const reveal = async () => {
    if (!target) return;
    try {
      const r = await revealIMAppSecret(target.id);
      setRevealedSecret(r.app_secret);
      setAppSecret(r.app_secret);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : (e as Error).message);
    }
  };

  const save = async () => {
    setErr(null);
    setBusy(true);
    try {
      const payload: IMAppPayload = {
        provider,
        mode,
        name: name.trim(),
        app_id: appID.trim(),
        // On edit, empty = keep current. On create, required.
        app_secret: appSecret.trim() || undefined,
        verify_token: verifyToken.trim() || undefined,
        encrypt_key: encryptKey.trim() || undefined,
        allow_from: provider === 'telegram' ? allowFrom.trim() || undefined : undefined,
        enabled,
      };
      if (isCreate) {
        await createIMApp(payload);
      } else if (target) {
        await updateIMApp(target.id, payload);
      }
      onSaved();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const meta = PROVIDER_META[provider];

  return (
    <Modal
      open
      onClose={onClose}
      size="lg"
      title={isCreate ? tr('新建 IM bot', 'New IM bot') : tr(`编辑 — ${target!.name}`, `Edit — ${target!.name}`)}
      footer={
        <>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-zinc-700 bg-zinc-900 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
          >
            {tr('取消', 'Cancel')}
          </button>
          <button
            type="button"
            onClick={save}
            disabled={busy}
            className="rounded-md bg-zinc-100 px-3 py-1.5 text-xs font-medium text-zinc-900 hover:bg-white disabled:opacity-50"
          >
            {busy ? tr('保存中…', 'Saving…') : tr('保存', 'Save')}
          </button>
        </>
      }
    >
      <div className="space-y-4 text-sm">
        {err && (
          <div className="rounded-md border border-red-500/40 bg-red-500/10 px-3 py-2 text-xs text-red-300">{err}</div>
        )}

        <div className="grid grid-cols-2 gap-3">
          <Field label={tr('平台', 'Provider')}>
            <select
              value={provider}
              onChange={(e) => {
                const p = e.target.value as IMProvider;
                setProvider(p);
                if (p === 'telegram') setMode('stream'); // telegram is stream-only
              }}
              disabled={!isCreate}
              className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1.5 text-xs text-zinc-100 focus:border-zinc-600 focus:outline-none disabled:opacity-50"
            >
              <option value="feishu">飞书 / Feishu</option>
              <option value="dingtalk">钉钉 / DingTalk</option>
              <option value="telegram">Telegram</option>
            </select>
          </Field>
          <Field label={tr('模式', 'Mode')} hint={mode === 'stream'
            ? tr('manager 主动 dial 长连接，无需公网回调。推荐。', 'Manager dials out via long connection — recommended.')
            : tr('平台 webhook 推到我们这边，需要公网回调 URL + encrypt_key。', 'Platform pushes webhooks to our public URL — needs encrypt_key.')}>
            <select
              value={mode}
              onChange={(e) => setMode(e.target.value as IMMode)}
              className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1.5 text-xs text-zinc-100 focus:border-zinc-600 focus:outline-none"
            >
              <option value="stream">stream (推荐 / recommended)</option>
              {provider !== 'telegram' && <option value="webhook">webhook</option>}
            </select>
          </Field>
        </div>

        <p className="rounded-md border border-zinc-800/60 bg-zinc-950/40 px-3 py-2 text-[11px] text-zinc-500">
          {tr(meta.hintZh, meta.hintEn)}
        </p>

        <Field label={tr('名称（仅展示）', 'Name (display only)')}>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={tr('如：运维群机器人', 'e.g. Ops Channel Bot')}
            className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1.5 text-xs text-zinc-100 focus:border-zinc-600 focus:outline-none"
          />
        </Field>

        <Field label="app_id" hint={tr('飞书 app_id (cli_xxx) / 钉钉 AppKey / Telegram bot 用户名', 'Feishu app_id (cli_xxx) / DingTalk AppKey / Telegram bot username')}>
          <input
            value={appID}
            onChange={(e) => setAppID(e.target.value)}
            placeholder="cli_a1b2c3d4e5f6"
            className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1.5 font-mono text-xs text-zinc-100 focus:border-zinc-600 focus:outline-none"
          />
        </Field>

        <Field label="app_secret" hint={isCreate
          ? tr('从平台开放后台拷贝（Telegram 填 BotFather 的 token）', 'Copy from the platform admin (Telegram: the BotFather token)')
          : tr('留空 = 保留现值；填了 = 覆盖', 'Empty = keep existing; filled = overwrite')}>
          <div className="flex items-center gap-2">
            <input
              type={revealedSecret ? 'text' : 'password'}
              value={appSecret}
              onChange={(e) => setAppSecret(e.target.value)}
              placeholder={isCreate ? tr('必填', 'Required') : tr('留空保留现值', 'Leave blank to keep current')}
              className="flex-1 rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1.5 font-mono text-xs text-zinc-100 focus:border-zinc-600 focus:outline-none"
            />
            {!isCreate && (
              <button
                type="button"
                onClick={revealedSecret ? () => { setRevealedSecret(null); setAppSecret(''); } : reveal}
                className="rounded-md border border-zinc-700 bg-zinc-900 px-2 py-1.5 text-zinc-300 hover:bg-zinc-800"
                title={revealedSecret ? tr('清空', 'Clear') : tr('查看', 'Reveal')}
              >
                {revealedSecret ? <EyeOff size={12} /> : <Eye size={12} />}
              </button>
            )}
          </div>
        </Field>

        {provider === 'telegram' && (
          <Field
            label={tr('allow_from（发送者白名单）', 'allow_from (sender allowlist)')}
            hint={tr(
              '必填。逗号分隔的 Telegram 数字 user id，只有名单内的人能和 bot 对话，其他人一律静默忽略。给自己发消息给 @userinfobot 可查到自己的 id。',
              'Required. Comma-separated numeric Telegram user IDs — only these may talk to the bot; everyone else is silently ignored. DM @userinfobot to find your own id.',
            )}
          >
            <input
              value={allowFrom}
              onChange={(e) => setAllowFrom(e.target.value)}
              placeholder="8211893274, 123456789"
              className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1.5 font-mono text-xs text-zinc-100 focus:border-zinc-600 focus:outline-none"
            />
          </Field>
        )}

        {mode === 'webhook' && provider === 'feishu' && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="verify_token" hint={tr('飞书事件订阅 verification token（可选）', 'Feishu event subscription verification token (optional)')}>
              <input
                value={verifyToken}
                onChange={(e) => setVerifyToken(e.target.value)}
                className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1.5 font-mono text-xs text-zinc-100 focus:border-zinc-600 focus:outline-none"
              />
            </Field>
            <Field label="encrypt_key" hint={tr('webhook 模式必填；事件加密 key', 'Required in webhook mode — event encryption key')}>
              <input
                value={encryptKey}
                onChange={(e) => setEncryptKey(e.target.value)}
                className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-2 py-1.5 font-mono text-xs text-zinc-100 focus:border-zinc-600 focus:outline-none"
              />
            </Field>
          </div>
        )}

        <label className="inline-flex items-center gap-2 text-xs text-zinc-300">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            className="h-3.5 w-3.5 rounded border-zinc-700 bg-zinc-900"
          />
          {tr('启用此 bot', 'Enable this bot')}
        </label>

        {mode === 'webhook' && (
          <div className="rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-[11px] text-amber-300">
            {tr(
              '⚠ Webhook 模式需要在平台开放后台填回调 URL：',
              '⚠ Webhook mode requires registering the callback URL in the platform admin:',
            )}
            <code className="ml-1 rounded bg-zinc-900 px-1 py-0.5 font-mono text-amber-200">
              https://&lt;your-host&gt;/api/v1/im/{provider}/events
            </code>
          </div>
        )}
      </div>
    </Modal>
  );
}

function Field({ label, hint, children }: { label: React.ReactNode; hint?: React.ReactNode; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs text-zinc-400">{label}</span>
      {children}
      {hint && <span className="mt-1 block text-[10px] text-zinc-500">{hint}</span>}
    </label>
  );
}

function DeleteConfirm({
  target,
  onClose,
  onDeleted,
}: {
  target: IMApp;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const { tr } = useI18n();
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    setErr(null);
    setBusy(true);
    try {
      await deleteIMApp(target.id);
      onDeleted();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      size="sm"
      title={tr(`删除 ${target.name}`, `Delete ${target.name}`)}
      footer={
        <>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-zinc-700 bg-zinc-900 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
          >
            {tr('取消', 'Cancel')}
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={busy}
            className="rounded-md bg-red-500 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-600 disabled:opacity-50"
          >
            {busy ? tr('删除中…', 'Deleting…') : tr('删除', 'Delete')}
          </button>
        </>
      }
    >
      <div className="space-y-2 text-xs text-zinc-300">
        {err && <div className="rounded-md border border-red-500/40 bg-red-500/10 px-3 py-2 text-red-300">{err}</div>}
        <p>{tr('删除后，关联的群 / DM 会话将不再可达。已有的 ongrid chat session 不会清除，仅断开映射。', 'After deletion, associated chats / DMs become unreachable. Existing ongrid chat sessions are kept, just unlinked.')}</p>
        <p className="text-zinc-500">app_id: <code className="font-mono">{target.app_id}</code></p>
      </div>
    </Modal>
  );
}
