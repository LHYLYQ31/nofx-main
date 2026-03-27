import { useEffect, useMemo, useState } from 'react'
import { BellRing, Plus, RefreshCw, Save, Trash2, Users } from 'lucide-react'
import { api } from '../lib/api'
import { notify } from '../lib/notify'
import type {
  BacktestShowcaseUserItem,
  SignalNotifyUserItem,
  Strategy,
  StrategyWebhookItem,
} from '../types'

export function StrategyWebhookConfigPage() {
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
      notify.error(err instanceof Error ? err.message : 'Failed to load webhook settings')
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
      notify.error('Please select a strategy')
      return
    }
    if (!url) {
      notify.error('Please input Discord webhook URL')
      return
    }
    try {
      await api.upsertAdminStrategyWebhook({
        strategy_id: strategyID,
        webhook_url: url,
        enabled: webhookEnabled,
      })
      notify.success('Strategy webhook saved')
      setWebhookURL('')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Failed to save strategy webhook')
    }
  }

  const removeWebhook = async (strategyID: string) => {
    try {
      await api.deleteAdminStrategyWebhook(strategyID)
      notify.success('Strategy webhook deleted')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Failed to delete strategy webhook')
    }
  }

  const saveNotifyUser = async () => {
    const email = emailInput.trim().toLowerCase()
    if (!email || !email.includes('@')) {
      notify.error('Please input a valid email')
      return
    }
    try {
      await api.upsertAdminSignalNotifyUser({ email, enabled: true })
      notify.success('Signal notify user saved')
      setEmailInput('')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Failed to save signal notify user')
    }
  }

  const toggleNotifyUser = async (item: SignalNotifyUserItem) => {
    try {
      await api.upsertAdminSignalNotifyUser({ email: item.email, enabled: !item.enabled })
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Failed to update signal notify user')
    }
  }

  const removeNotifyUser = async (email: string) => {
    try {
      await api.deleteAdminSignalNotifyUser(email)
      notify.success('Signal notify user deleted')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Failed to delete signal notify user')
    }
  }

  const saveShowcaseUser = async () => {
    const email = showcaseEmailInput.trim().toLowerCase()
    if (!email || !email.includes('@')) {
      notify.error('Please input a valid showcase email')
      return
    }
    try {
      await api.upsertAdminBacktestShowcaseUser({ email, enabled: true })
      notify.success('Backtest showcase user saved')
      setShowcaseEmailInput('')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Failed to save backtest showcase user')
    }
  }

  const toggleShowcaseUser = async (item: BacktestShowcaseUserItem) => {
    try {
      await api.upsertAdminBacktestShowcaseUser({ email: item.email, enabled: !item.enabled })
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Failed to update backtest showcase user')
    }
  }

  const removeShowcaseUser = async (email: string) => {
    try {
      await api.deleteAdminBacktestShowcaseUser(email)
      notify.success('Backtest showcase user deleted')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Failed to delete backtest showcase user')
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
            Strategy Discord Webhook
          </h2>
          <button
            type="button"
            onClick={() => void loadAll()}
            className="flex items-center gap-2 rounded border px-3 py-2 text-sm"
            style={{ borderColor: '#2B3139', color: '#AEB4BC' }}
          >
            <RefreshCw className="h-4 w-4" />
            Refresh
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
            Enabled
          </label>
        </div>

        <button
          type="button"
          onClick={() => void saveWebhook()}
          className="flex items-center gap-2 rounded border px-3 py-2 text-sm"
          style={{ borderColor: '#F0B90B', color: '#F0B90B' }}
        >
          <Save className="h-4 w-4" />
          Save Strategy Webhook
        </button>

        <div className="mt-4 space-y-2">
          {loading ? (
            <div style={{ color: '#848E9C' }}>Loading...</div>
          ) : webhooks.length === 0 ? (
            <div style={{ color: '#848E9C' }}>No strategy webhook settings</div>
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
                    {item.enabled ? 'Enabled' : 'Disabled'}
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
          Signal Notify User List
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
            Add
          </button>
        </div>
        <div className="space-y-2">
          {users.length === 0 ? (
            <div style={{ color: '#848E9C' }}>No notify users</div>
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
                    {u.enabled ? 'Enabled' : 'Disabled'}
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
          Backtest Showcase Accounts
        </h2>
        <p className="mb-4 text-sm" style={{ color: '#848E9C' }}>
          Showcase page will aggregate runs from all enabled accounts below.
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
            Add
          </button>
        </div>
        <div className="space-y-2">
          {showcaseUsers.length === 0 ? (
            <div style={{ color: '#848E9C' }}>No backtest showcase accounts</div>
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
                    {u.enabled ? 'Enabled' : 'Disabled'}
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
