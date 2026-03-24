import { useEffect, useMemo, useState } from 'react'
import { Save, Trash2, Plus, RefreshCw, BellRing } from 'lucide-react'
import { api } from '../lib/api'
import { notify } from '../lib/notify'
import type { SignalNotifyUserItem, Strategy, StrategyWebhookItem } from '../types'

export function StrategyWebhookConfigPage() {
  const [loading, setLoading] = useState(true)
  const [strategies, setStrategies] = useState<Strategy[]>([])
  const [webhooks, setWebhooks] = useState<StrategyWebhookItem[]>([])
  const [users, setUsers] = useState<SignalNotifyUserItem[]>([])

  const [selectedStrategyID, setSelectedStrategyID] = useState('')
  const [webhookURL, setWebhookURL] = useState('')
  const [webhookEnabled, setWebhookEnabled] = useState(true)
  const [emailInput, setEmailInput] = useState('')

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
      const [st, hooks, notifyUsers] = await Promise.all([
        api.getAvailableStrategies(),
        api.getAdminStrategyWebhooks(),
        api.getAdminSignalNotifyUsers(),
      ])
      setStrategies(st)
      setWebhooks(hooks)
      setUsers(notifyUsers)
      if (!selectedStrategyID && st.length > 0) {
        setSelectedStrategyID(st[0].id)
      }
    } catch (err) {
      notify.error(err instanceof Error ? err.message : 'Failed to load webhook config')
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
      notify.error('请选择策略')
      return
    }
    if (!url) {
      notify.error('请输入 Discord webhook URL')
      return
    }
    try {
      await api.upsertAdminStrategyWebhook({
        strategy_id: strategyID,
        webhook_url: url,
        enabled: webhookEnabled,
      })
      notify.success('策略 webhook 已保存')
      setWebhookURL('')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : '保存失败')
    }
  }

  const removeWebhook = async (strategyID: string) => {
    try {
      await api.deleteAdminStrategyWebhook(strategyID)
      notify.success('策略 webhook 已删除')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  const saveUser = async () => {
    const email = emailInput.trim().toLowerCase()
    if (!email || !email.includes('@')) {
      notify.error('请输入有效邮箱')
      return
    }
    try {
      await api.upsertAdminSignalNotifyUser({ email, enabled: true })
      notify.success('通知用户已保存')
      setEmailInput('')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : '保存失败')
    }
  }

  const toggleUser = async (item: SignalNotifyUserItem) => {
    try {
      await api.upsertAdminSignalNotifyUser({ email: item.email, enabled: !item.enabled })
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : '更新失败')
    }
  }

  const removeUser = async (email: string) => {
    try {
      await api.deleteAdminSignalNotifyUser(email)
      notify.success('通知用户已删除')
      await loadAll()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <div className="max-w-6xl mx-auto px-4 py-6 space-y-6">
      <div className="rounded-xl border p-4" style={{ background: '#11161E', borderColor: '#2B3139' }}>
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold" style={{ color: '#EAECEF' }}>策略 Discord Webhook 配置</h2>
          <button
            type="button"
            onClick={() => void loadAll()}
            className="px-3 py-2 rounded border text-sm flex items-center gap-2"
            style={{ borderColor: '#2B3139', color: '#AEB4BC' }}
          >
            <RefreshCw className="w-4 h-4" />
            刷新
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-4 gap-3 mb-4">
          <select
            value={selectedStrategyID}
            onChange={(e) => setSelectedStrategyID(e.target.value)}
            className="px-3 py-2 rounded border bg-transparent"
            style={{ borderColor: '#2B3139', color: '#EAECEF' }}
          >
            {strategies.map((st) => (
              <option key={st.id} value={st.id}>{st.name}</option>
            ))}
          </select>
          <input
            value={webhookURL}
            onChange={(e) => setWebhookURL(e.target.value)}
            placeholder="https://discord.com/api/webhooks/..."
            className="px-3 py-2 rounded border md:col-span-2"
            style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
          />
          <label className="flex items-center gap-2 text-sm" style={{ color: '#AEB4BC' }}>
            <input
              type="checkbox"
              checked={webhookEnabled}
              onChange={(e) => setWebhookEnabled(e.target.checked)}
            />
            启用
          </label>
        </div>

        <button
          type="button"
          onClick={() => void saveWebhook()}
          className="px-3 py-2 rounded border text-sm flex items-center gap-2"
          style={{ borderColor: '#F0B90B', color: '#F0B90B' }}
        >
          <Save className="w-4 h-4" />
          保存策略 Webhook
        </button>

        <div className="mt-4 space-y-2">
          {loading ? (
            <div style={{ color: '#848E9C' }}>加载中...</div>
          ) : webhooks.length === 0 ? (
            <div style={{ color: '#848E9C' }}>暂无策略 webhook 配置</div>
          ) : webhooks.map((item) => (
            <div key={item.strategy_id} className="rounded border p-3 flex items-center justify-between" style={{ borderColor: '#2B3139' }}>
              <div className="min-w-0">
                <div style={{ color: '#EAECEF' }}>
                  {item.strategy_name || strategyNameByID.get(item.strategy_id) || item.strategy_id}
                </div>
                <div className="text-xs truncate" style={{ color: '#848E9C' }}>{item.webhook_url}</div>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-xs" style={{ color: item.enabled ? '#0ECB81' : '#F6465D' }}>
                  {item.enabled ? '已启用' : '已禁用'}
                </span>
                <button
                  type="button"
                  onClick={() => void removeWebhook(item.strategy_id)}
                  className="p-2 rounded border"
                  style={{ borderColor: '#2B3139', color: '#F6465D' }}
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="rounded-xl border p-4" style={{ background: '#11161E', borderColor: '#2B3139' }}>
        <h2 className="text-lg font-semibold mb-4 flex items-center gap-2" style={{ color: '#EAECEF' }}>
          <BellRing className="w-5 h-5" />
          信号通知用户白名单（邮箱）
        </h2>
        <div className="flex gap-3 mb-4">
          <input
            value={emailInput}
            onChange={(e) => setEmailInput(e.target.value)}
            placeholder="example@gmail.com"
            className="px-3 py-2 rounded border flex-1"
            style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
          />
          <button
            type="button"
            onClick={() => void saveUser()}
            className="px-3 py-2 rounded border text-sm flex items-center gap-2"
            style={{ borderColor: '#F0B90B', color: '#F0B90B' }}
          >
            <Plus className="w-4 h-4" />
            添加
          </button>
        </div>
        <div className="space-y-2">
          {users.length === 0 ? (
            <div style={{ color: '#848E9C' }}>暂无白名单用户</div>
          ) : users.map((u) => (
            <div key={u.email} className="rounded border p-3 flex items-center justify-between" style={{ borderColor: '#2B3139' }}>
              <span style={{ color: '#EAECEF' }}>{u.email}</span>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => void toggleUser(u)}
                  className="px-3 py-1 rounded border text-xs"
                  style={{ borderColor: '#2B3139', color: u.enabled ? '#0ECB81' : '#F6465D' }}
                >
                  {u.enabled ? '已启用' : '已禁用'}
                </button>
                <button
                  type="button"
                  onClick={() => void removeUser(u.email)}
                  className="p-2 rounded border"
                  style={{ borderColor: '#2B3139', color: '#F6465D' }}
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
