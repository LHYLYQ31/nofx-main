import { useEffect, useMemo, useState } from 'react'
import { BellRing, Plus, RefreshCw, Save, Trash2, Users } from 'lucide-react'
import { useLanguage } from '../contexts/LanguageContext'
import { api } from '../lib/api'
import { notify } from '../lib/notify'
import type {
  BacktestShowcaseUserItem,
  SignalNotifyUserItem,
  Strategy,
  StrategyWebhookItem,
} from '../types'

export function StrategyWebhookConfigPage() {
  const { language } = useLanguage()
  const tr = (zh: string, en: string, id: string = en) =>
    language === 'zh' ? zh : language === 'id' ? id : en

  const [loading, setLoading] = useState(true)
  const [strategies, setStrategies] = useState<Strategy[]>([])
  const [webhooks, setWebhooks] = useState<StrategyWebhookItem[]>([])
  const [users, setUsers] = useState<SignalNotifyUserItem[]>([])
  const [showcaseUsers, setShowcaseUsers] = useState<BacktestShowcaseUserItem[]>([])

  const [selectedStrategyID, setSelectedStrategyID] = useState('')
  const [webhookURL, setWebhookURL] = useState('')
  const [webhookEnabled, setWebhookEnabled] = useState(true)
  const [emailInput, setEmailInput] = useState('')
  const [showcaseEmailInput, setShowcaseEmailInput] = useState('')

  const strategyNameByID = useMemo(() => {
    const map = new Map<string, string>()
    for (const st of strategies) {
      map.set(st.id, st.name)
    }
    return map
  }, [strategies])

  const loadAll = async () => {
    setLoading(true)
    try {
      const [st, hooks, notifyUsers, showcase] = await Promise.all([
        api.getAvailableStrategies(),
        api.getAdminStrategyWebhooks(),
        api.getAdminSignalNotifyUsers(),
        api.getAdminBacktestShowcaseUsers(),
      ])
      setStrategies(st)
      setWebhooks(hooks)
      setUsers(notifyUsers)
      setShowcaseUsers(showcase)
      if (!selectedStrategyID && st.length > 0) {
        setSelectedStrategyID(st[0].id)
      }
    } catch (err) {
      notify.error(err instanceof Error ? err.message : tr('加载 Webhook 配置失败', 'Failed to load webhook settings', 'Gagal memuat pengaturan webhook'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadAll()
  }, [])

  const saveWebhook = async () => {
    const strategyID = selectedStrategyID.trim()
    const url = webhookURL.trim()
    if (!strategyID) {
      notify.error(tr('请选择策略', 'Please select a strategy', 'Silakan pilih strategi'))
      return
    }
    if (!url) {
      notify.error(tr('请输入 Discord webhook URL', 'Please input Discord webhook URL', 'Silakan isi URL webhook Discord'))
      return
    }
    try {
      await api.upsertAdminStrategyWebhook({
        strategy_id: strategyID,
        webhook_url: url,
        enabled: webhookEnabled,
      })
      notify.success(tr('策略 Webhook 已保存', 'Strategy webhook saved', 'Webhook strategi tersimpan'))
      setWebhookURL('')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : tr('保存策略 Webhook 失败', 'Failed to save strategy webhook', 'Gagal menyimpan webhook strategi'))
    }
  }

  const removeWebhook = async (strategyID: string) => {
    try {
      await api.deleteAdminStrategyWebhook(strategyID)
      notify.success(tr('策略 Webhook 已删除', 'Strategy webhook deleted', 'Webhook strategi dihapus'))
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : tr('删除策略 Webhook 失败', 'Failed to delete strategy webhook', 'Gagal menghapus webhook strategi'))
    }
  }

  const saveNotifyUser = async () => {
    const email = emailInput.trim().toLowerCase()
    if (!email || !email.includes('@')) {
      notify.error(tr('请输入有效邮箱', 'Please input a valid email', 'Silakan isi email yang valid'))
      return
    }
    try {
      await api.upsertAdminSignalNotifyUser({ email, enabled: true })
      notify.success(tr('信号通知用户已保存', 'Signal notify user saved', 'Pengguna notifikasi sinyal tersimpan'))
      setEmailInput('')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : tr('保存信号通知用户失败', 'Failed to save signal notify user', 'Gagal menyimpan pengguna notifikasi sinyal'))
    }
  }

  const toggleNotifyUser = async (item: SignalNotifyUserItem) => {
    try {
      await api.upsertAdminSignalNotifyUser({ email: item.email, enabled: !item.enabled })
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : tr('更新信号通知用户失败', 'Failed to update signal notify user', 'Gagal memperbarui pengguna notifikasi sinyal'))
    }
  }

  const removeNotifyUser = async (email: string) => {
    try {
      await api.deleteAdminSignalNotifyUser(email)
      notify.success(tr('信号通知用户已删除', 'Signal notify user deleted', 'Pengguna notifikasi sinyal dihapus'))
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : tr('删除信号通知用户失败', 'Failed to delete signal notify user', 'Gagal menghapus pengguna notifikasi sinyal'))
    }
  }

  const saveShowcaseUser = async () => {
    const email = showcaseEmailInput.trim().toLowerCase()
    if (!email || !email.includes('@')) {
      notify.error(tr('请输入有效展示邮箱', 'Please input a valid showcase email', 'Silakan isi email showcase yang valid'))
      return
    }
    try {
      await api.upsertAdminBacktestShowcaseUser({ email, enabled: true })
      notify.success(tr('回测展示账号已保存', 'Backtest showcase user saved', 'Akun showcase backtest tersimpan'))
      setShowcaseEmailInput('')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : tr('保存回测展示账号失败', 'Failed to save backtest showcase user', 'Gagal menyimpan akun showcase backtest'))
    }
  }

  const toggleShowcaseUser = async (item: BacktestShowcaseUserItem) => {
    try {
      await api.upsertAdminBacktestShowcaseUser({ email: item.email, enabled: !item.enabled })
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : tr('更新回测展示账号失败', 'Failed to update backtest showcase user', 'Gagal memperbarui akun showcase backtest'))
    }
  }

  const removeShowcaseUser = async (email: string) => {
    try {
      await api.deleteAdminBacktestShowcaseUser(email)
      notify.success(tr('回测展示账号已删除', 'Backtest showcase user deleted', 'Akun showcase backtest dihapus'))
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : tr('删除回测展示账号失败', 'Failed to delete backtest showcase user', 'Gagal menghapus akun showcase backtest'))
    }
  }

  return (
    <div className="mx-auto max-w-6xl space-y-6 px-4 py-6">
      <section
        className="rounded-xl border p-4"
        style={{ background: '#11161E', borderColor: '#2B3139' }}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold" style={{ color: '#EAECEF' }}>
            {tr('策略 Discord Webhook', 'Strategy Discord Webhook', 'Webhook Discord Strategi')}
          </h2>
          <button
            type="button"
            onClick={() => void loadAll()}
            className="flex items-center gap-2 rounded border px-3 py-2 text-sm"
            style={{ borderColor: '#2B3139', color: '#AEB4BC' }}
          >
            <RefreshCw className="h-4 w-4" />
            {tr('刷新', 'Refresh', 'Muat Ulang')}
          </button>
        </div>

        <div className="mb-4 grid grid-cols-1 gap-3 md:grid-cols-4">
          <select
            value={selectedStrategyID}
            onChange={(e) => setSelectedStrategyID(e.target.value)}
            className="rounded border bg-transparent px-3 py-2"
            style={{ borderColor: '#2B3139', color: '#EAECEF' }}
          >
            {strategies.map((st) => (
              <option key={st.id} value={st.id}>
                {st.name}
              </option>
            ))}
          </select>
          <input
            value={webhookURL}
            onChange={(e) => setWebhookURL(e.target.value)}
            placeholder="https://discord.com/api/webhooks/..."
            className="rounded border px-3 py-2 md:col-span-2"
            style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
          />
          <label className="flex items-center gap-2 text-sm" style={{ color: '#AEB4BC' }}>
            <input
              type="checkbox"
              checked={webhookEnabled}
              onChange={(e) => setWebhookEnabled(e.target.checked)}
            />
            {tr('启用', 'Enabled', 'Aktif')}
          </label>
        </div>

        <button
          type="button"
          onClick={() => void saveWebhook()}
          className="flex items-center gap-2 rounded border px-3 py-2 text-sm"
          style={{ borderColor: '#F0B90B', color: '#F0B90B' }}
        >
          <Save className="h-4 w-4" />
          {tr('保存策略 Webhook', 'Save Strategy Webhook', 'Simpan Webhook Strategi')}
        </button>

        <div className="mt-4 space-y-2">
          {loading ? (
            <div style={{ color: '#848E9C' }}>{tr('加载中...', 'Loading...', 'Memuat...')}</div>
          ) : webhooks.length === 0 ? (
            <div style={{ color: '#848E9C' }}>{tr('暂无策略 Webhook 配置', 'No strategy webhook settings', 'Belum ada pengaturan webhook strategi')}</div>
          ) : (
            webhooks.map((item) => (
              <div
                key={item.strategy_id}
                className="flex items-center justify-between rounded border p-3"
                style={{ borderColor: '#2B3139' }}
              >
                <div className="min-w-0">
                  <div style={{ color: '#EAECEF' }}>
                    {item.strategy_name || strategyNameByID.get(item.strategy_id) || item.strategy_id}
                  </div>
                  <div className="truncate text-xs" style={{ color: '#848E9C' }}>
                    {item.webhook_url}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-xs" style={{ color: item.enabled ? '#0ECB81' : '#F6465D' }}>
                    {item.enabled ? tr('启用', 'Enabled', 'Aktif') : tr('禁用', 'Disabled', 'Nonaktif')}
                  </span>
                  <button
                    type="button"
                    onClick={() => void removeWebhook(item.strategy_id)}
                    className="rounded border p-2"
                    style={{ borderColor: '#2B3139', color: '#F6465D' }}
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </section>

      <section
        className="rounded-xl border p-4"
        style={{ background: '#11161E', borderColor: '#2B3139' }}
      >
        <h2 className="mb-4 flex items-center gap-2 text-lg font-semibold" style={{ color: '#EAECEF' }}>
          <BellRing className="h-5 w-5" />
          {tr('信号通知用户列表', 'Signal Notify User List', 'Daftar Pengguna Notifikasi Sinyal')}
        </h2>
        <div className="mb-4 flex gap-3">
          <input
            value={emailInput}
            onChange={(e) => setEmailInput(e.target.value)}
            placeholder="example@gmail.com"
            className="flex-1 rounded border px-3 py-2"
            style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
          />
          <button
            type="button"
            onClick={() => void saveNotifyUser()}
            className="flex items-center gap-2 rounded border px-3 py-2 text-sm"
            style={{ borderColor: '#F0B90B', color: '#F0B90B' }}
          >
            <Plus className="h-4 w-4" />
            {tr('添加', 'Add', 'Tambah')}
          </button>
        </div>
        <div className="space-y-2">
          {users.length === 0 ? (
            <div style={{ color: '#848E9C' }}>{tr('暂无通知用户', 'No notify users', 'Belum ada pengguna notifikasi')}</div>
          ) : (
            users.map((u) => (
              <div
                key={u.email}
                className="flex items-center justify-between rounded border p-3"
                style={{ borderColor: '#2B3139' }}
              >
                <span style={{ color: '#EAECEF' }}>{u.email}</span>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => void toggleNotifyUser(u)}
                    className="rounded border px-3 py-1 text-xs"
                    style={{ borderColor: '#2B3139', color: u.enabled ? '#0ECB81' : '#F6465D' }}
                  >
                    {u.enabled ? tr('启用', 'Enabled', 'Aktif') : tr('禁用', 'Disabled', 'Nonaktif')}
                  </button>
                  <button
                    type="button"
                    onClick={() => void removeNotifyUser(u.email)}
                    className="rounded border p-2"
                    style={{ borderColor: '#2B3139', color: '#F6465D' }}
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </section>

      <section
        className="rounded-xl border p-4"
        style={{ background: '#11161E', borderColor: '#2B3139' }}
      >
        <h2 className="mb-2 flex items-center gap-2 text-lg font-semibold" style={{ color: '#EAECEF' }}>
          <Users className="h-5 w-5" />
          {tr('回测展示账号', 'Backtest Showcase Accounts', 'Akun Showcase Backtest')}
        </h2>
        <p className="mb-4 text-sm" style={{ color: '#848E9C' }}>
          {tr(
            '展示页会聚合下方所有已启用账号的运行结果。',
            'Showcase page will aggregate runs from all enabled accounts below.',
            'Halaman showcase akan menggabungkan run dari semua akun aktif di bawah ini.'
          )}
        </p>
        <div className="mb-4 flex gap-3">
          <input
            value={showcaseEmailInput}
            onChange={(e) => setShowcaseEmailInput(e.target.value)}
            placeholder="showcase-account@gmail.com"
            className="flex-1 rounded border px-3 py-2"
            style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
          />
          <button
            type="button"
            onClick={() => void saveShowcaseUser()}
            className="flex items-center gap-2 rounded border px-3 py-2 text-sm"
            style={{ borderColor: '#F0B90B', color: '#F0B90B' }}
          >
            <Plus className="h-4 w-4" />
            {tr('添加', 'Add', 'Tambah')}
          </button>
        </div>
        <div className="space-y-2">
          {showcaseUsers.length === 0 ? (
            <div style={{ color: '#848E9C' }}>{tr('暂无回测展示账号', 'No backtest showcase accounts', 'Belum ada akun showcase backtest')}</div>
          ) : (
            showcaseUsers.map((u) => (
              <div
                key={u.email}
                className="flex items-center justify-between rounded border p-3"
                style={{ borderColor: '#2B3139' }}
              >
                <span style={{ color: '#EAECEF' }}>{u.email}</span>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => void toggleShowcaseUser(u)}
                    className="rounded border px-3 py-1 text-xs"
                    style={{ borderColor: '#2B3139', color: u.enabled ? '#0ECB81' : '#F6465D' }}
                  >
                    {u.enabled ? tr('启用', 'Enabled', 'Aktif') : tr('禁用', 'Disabled', 'Nonaktif')}
                  </button>
                  <button
                    type="button"
                    onClick={() => void removeShowcaseUser(u.email)}
                    className="rounded border p-2"
                    style={{ borderColor: '#2B3139', color: '#F6465D' }}
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </section>
    </div>
  )
}
