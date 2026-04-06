import { useEffect, useMemo, useState } from 'react'
import { CreditCard, RefreshCw, Save, Shield } from 'lucide-react'
import { useLanguage } from '../contexts/LanguageContext'
import { api } from '../lib/api'
import { notify } from '../lib/notify'
import type { MembershipPlanItem, PaymentProviderConfigItem } from '../types'

type ProviderForm = {
  id?: string
  environment: string
  display_name: string
  base_url: string
  key_id: string
  secret_key: string
  webhook_secret: string
  enabled: boolean
  is_default: boolean
  version?: number
}

const emptyProviderForm = (): ProviderForm => ({
  environment: 'production',
  display_name: 'Infini Production',
  base_url: 'https://openapi.infini.money',
  key_id: '',
  secret_key: '',
  webhook_secret: '',
  enabled: true,
  is_default: true,
})

function centsToDisplay(cents: number): string {
  return (Math.max(0, cents) / 100).toFixed(2)
}

function displayToCents(value: string): number {
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed < 0) {
    return 0
  }
  return Math.round(parsed * 100)
}

export function PaymentConfigPage() {
  const { language } = useLanguage()
  const tr = (_zh: string, en: string, id: string = en) =>
    language === 'id' ? id : en

  const [loading, setLoading] = useState(true)
  const [savingProvider, setSavingProvider] = useState(false)
  const [savingPlanCode, setSavingPlanCode] = useState<string>('')

  const [providerConfigs, setProviderConfigs] = useState<PaymentProviderConfigItem[]>([])
  const [selectedProviderConfigID, setSelectedProviderConfigID] = useState('')
  const [providerForm, setProviderForm] = useState<ProviderForm>(emptyProviderForm)

  const [plans, setPlans] = useState<MembershipPlanItem[]>([])

  const selectedProvider = useMemo(
    () => providerConfigs.find((item) => item.id === selectedProviderConfigID),
    [providerConfigs, selectedProviderConfigID]
  )

  const sortedPlans = useMemo(() => {
    return [...plans].sort((a, b) => a.sort_order - b.sort_order || a.price_cents - b.price_cents)
  }, [plans])

  const fillProviderForm = (item?: PaymentProviderConfigItem) => {
    if (!item) {
      setProviderForm(emptyProviderForm())
      return
    }
    setProviderForm({
      id: item.id,
      environment: item.environment || 'production',
      display_name: item.display_name || '',
      base_url: item.base_url || '',
      key_id: item.key_id || '',
      secret_key: '',
      webhook_secret: '',
      enabled: !!item.enabled,
      is_default: !!item.is_default,
      version: item.version,
    })
  }

  const loadAll = async () => {
    setLoading(true)
    try {
      const [configItems, planItems] = await Promise.all([
        api.getAdminPaymentProviderConfigs('infini'),
        api.getAdminMembershipPlans(),
      ])
      setProviderConfigs(configItems)
      setPlans(planItems)

      const target =
        configItems.find((item) => item.id === selectedProviderConfigID) ||
        configItems.find((item) => item.is_default) ||
        configItems[0]

      if (target) {
        setSelectedProviderConfigID(target.id)
      }
      fillProviderForm(target)
    } catch (error) {
      notify.error(
        error instanceof Error
          ? error.message
          : tr('加载支付配置失败', 'Failed to load payment settings', 'Gagal memuat pengaturan pembayaran')
      )
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadAll()
  }, [])

  const onSelectProvider = (id: string) => {
    setSelectedProviderConfigID(id)
    const item = providerConfigs.find((cfg) => cfg.id === id)
    fillProviderForm(item)
  }

  const saveProvider = async () => {
    const baseURL = providerForm.base_url.trim()
    const keyID = providerForm.key_id.trim()
    if (!baseURL || !keyID) {
      notify.error(tr('Base URL 和 Key ID 必填', 'Base URL and Key ID are required', 'Base URL dan Key ID wajib diisi'))
      return
    }

    if (!providerForm.id && !providerForm.secret_key.trim()) {
      notify.error(tr('首次创建必须填写 Secret Key', 'Secret Key is required for new config', 'Secret Key wajib untuk konfigurasi baru'))
      return
    }

    if (!providerForm.id && !providerForm.webhook_secret.trim()) {
      notify.error(tr('首次创建必须填写 Webhook Secret', 'Webhook Secret is required for new config', 'Webhook Secret wajib untuk konfigurasi baru'))
      return
    }

    setSavingProvider(true)
    try {
      await api.upsertAdminPaymentProviderConfig('infini', {
        id: providerForm.id,
        environment: providerForm.environment,
        display_name: providerForm.display_name,
        base_url: baseURL,
        key_id: keyID,
        secret_key: providerForm.secret_key.trim() || undefined,
        webhook_secret: providerForm.webhook_secret.trim() || undefined,
        enabled: providerForm.enabled,
        is_default: providerForm.is_default,
        version: providerForm.version,
      })
      notify.success(tr('支付配置已保存', 'Payment provider config saved', 'Konfigurasi pembayaran tersimpan'))
      await loadAll()
    } catch (error) {
      notify.error(
        error instanceof Error
          ? error.message
          : tr('保存支付配置失败', 'Failed to save payment provider config', 'Gagal menyimpan konfigurasi pembayaran')
      )
    } finally {
      setSavingProvider(false)
    }
  }

  const updatePlan = <K extends keyof MembershipPlanItem>(code: string, key: K, value: MembershipPlanItem[K]) => {
    setPlans((prev) =>
      prev.map((plan) => (plan.code === code ? { ...plan, [key]: value } : plan))
    )
  }

  const savePlan = async (plan: MembershipPlanItem) => {
    if (!plan.name.trim()) {
      notify.error(tr('套餐名称不能为空', 'Plan name is required', 'Nama paket wajib diisi'))
      return
    }

    try {
      JSON.parse(plan.entitlements || '{}')
    } catch {
      notify.error(
        tr(
          `套餐 ${plan.code} 的权益 JSON 格式错误`,
          `Entitlements JSON is invalid for plan ${plan.code}`,
          `JSON entitlements tidak valid untuk paket ${plan.code}`
        )
      )
      return
    }

    setSavingPlanCode(plan.code)
    try {
      await api.upsertAdminMembershipPlan(plan.code, {
        name: plan.name,
        description: plan.description,
        price_cents: plan.price_cents,
        currency: plan.currency,
        billing_cycle: plan.billing_cycle,
        revenue_share_bps: plan.revenue_share_bps,
        seat_limit: plan.seat_limit,
        enabled: plan.enabled,
        sort_order: plan.sort_order,
        entitlements: plan.entitlements,
      })
      notify.success(
        tr(
          `套餐 ${plan.code} 已保存`,
          `Membership plan ${plan.code} saved`,
          `Paket membership ${plan.code} tersimpan`
        )
      )
      await loadAll()
    } catch (error) {
      notify.error(
        error instanceof Error
          ? error.message
          : tr('保存会员套餐失败', 'Failed to save membership plan', 'Gagal menyimpan paket membership')
      )
    } finally {
      setSavingPlanCode('')
    }
  }

  return (
    <div className="mx-auto max-w-6xl space-y-6 px-4 py-6">
      <section className="rounded-xl border p-4" style={{ background: '#11161E', borderColor: '#2B3139' }}>
        <div className="mb-4 flex items-center justify-between">
          <h2 className="flex items-center gap-2 text-lg font-semibold" style={{ color: '#EAECEF' }}>
            <CreditCard className="h-5 w-5" />
            {tr('支付参数配置（Infini）', 'Payment Provider Config (Infini)', 'Konfigurasi Payment Provider (Infini)')}
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

        <div className="mb-4 grid grid-cols-1 gap-3 md:grid-cols-3">
          <select
            value={selectedProviderConfigID}
            onChange={(e) => onSelectProvider(e.target.value)}
            className="rounded border bg-transparent px-3 py-2"
            style={{ borderColor: '#2B3139', color: '#EAECEF' }}
          >
            <option value="">{tr('新建配置', 'New Config', 'Konfigurasi Baru')}</option>
            {providerConfigs.map((item) => (
              <option key={item.id} value={item.id}>
                {`${item.display_name || item.environment} v${item.version}${item.is_default ? ' (default)' : ''}`}
              </option>
            ))}
          </select>

          <input
            value={providerForm.display_name}
            onChange={(e) => setProviderForm((prev) => ({ ...prev, display_name: e.target.value }))}
            placeholder={tr('显示名称', 'Display name', 'Nama tampilan')}
            className="rounded border px-3 py-2"
            style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
          />

          <select
            value={providerForm.environment}
            onChange={(e) => setProviderForm((prev) => ({ ...prev, environment: e.target.value }))}
            className="rounded border bg-transparent px-3 py-2"
            style={{ borderColor: '#2B3139', color: '#EAECEF' }}
          >
            <option value="production">production</option>
            <option value="sandbox">sandbox</option>
          </select>

          <input
            value={providerForm.base_url}
            onChange={(e) => setProviderForm((prev) => ({ ...prev, base_url: e.target.value }))}
            placeholder="https://openapi.infini.money"
            className="rounded border px-3 py-2 md:col-span-2"
            style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
          />

          <input
            value={providerForm.key_id}
            onChange={(e) => setProviderForm((prev) => ({ ...prev, key_id: e.target.value }))}
            placeholder="key_id"
            className="rounded border px-3 py-2"
            style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
          />

          <input
            value={providerForm.secret_key}
            onChange={(e) => setProviderForm((prev) => ({ ...prev, secret_key: e.target.value }))}
            placeholder={
              selectedProvider?.has_secret_key
                ? tr('留空则沿用旧 Secret Key', 'Leave blank to keep existing Secret Key', 'Kosongkan untuk tetap memakai Secret Key lama')
                : 'secret_key'
            }
            className="rounded border px-3 py-2 md:col-span-2"
            style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
          />

          <input
            value={providerForm.webhook_secret}
            onChange={(e) => setProviderForm((prev) => ({ ...prev, webhook_secret: e.target.value }))}
            placeholder={
              selectedProvider?.has_webhook_secret
                ? tr('留空则沿用旧 Webhook Secret', 'Leave blank to keep existing Webhook Secret', 'Kosongkan untuk tetap memakai Webhook Secret lama')
                : 'webhook_secret'
            }
            className="rounded border px-3 py-2 md:col-span-2"
            style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
          />

          <div className="flex items-center gap-4 text-sm" style={{ color: '#AEB4BC' }}>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={providerForm.enabled}
                onChange={(e) => setProviderForm((prev) => ({ ...prev, enabled: e.target.checked }))}
              />
              {tr('启用', 'Enabled', 'Aktif')}
            </label>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={providerForm.is_default}
                onChange={(e) => setProviderForm((prev) => ({ ...prev, is_default: e.target.checked }))}
              />
              {tr('设为默认', 'Set default', 'Jadikan default')}
            </label>
          </div>
        </div>

        <div className="mb-4 text-xs" style={{ color: '#848E9C' }}>
          {tr(
            '提示：支付参数存数据库。签名密钥加密存储；下单时后端会动态加载当前启用配置。',
            'Tip: Payment params are stored in DB. Secrets are encrypted; runtime uses active config dynamically.',
            'Tips: Parameter pembayaran disimpan di DB. Secret dienkripsi; runtime memuat konfigurasi aktif secara dinamis.'
          )}
        </div>

        <button
          type="button"
          onClick={() => void saveProvider()}
          disabled={savingProvider}
          className="flex items-center gap-2 rounded border px-3 py-2 text-sm disabled:opacity-60"
          style={{ borderColor: '#F0B90B', color: '#F0B90B' }}
        >
          <Save className="h-4 w-4" />
          {savingProvider
            ? tr('保存中...', 'Saving...', 'Menyimpan...')
            : tr('保存支付配置', 'Save Payment Config', 'Simpan Konfigurasi Pembayaran')}
        </button>

        <div className="mt-4 space-y-2">
          {loading ? (
            <div style={{ color: '#848E9C' }}>{tr('加载中...', 'Loading...', 'Memuat...')}</div>
          ) : providerConfigs.length === 0 ? (
            <div style={{ color: '#848E9C' }}>
              {tr('暂无支付配置，请先创建一条。', 'No payment configs yet. Create your first one.', 'Belum ada konfigurasi pembayaran. Buat konfigurasi pertama.')}
            </div>
          ) : (
            providerConfigs.map((item) => (
              <div
                key={item.id}
                className="flex items-center justify-between rounded border p-3"
                style={{ borderColor: '#2B3139' }}
              >
                <div className="min-w-0">
                  <div style={{ color: '#EAECEF' }}>{`${item.display_name || item.environment} (v${item.version})`}</div>
                  <div className="truncate text-xs" style={{ color: '#848E9C' }}>{`${item.base_url} | key_id: ${item.key_id}`}</div>
                </div>
                <div className="flex items-center gap-2 text-xs">
                  <span style={{ color: item.enabled ? '#0ECB81' : '#F6465D' }}>
                    {item.enabled ? tr('启用', 'Enabled', 'Aktif') : tr('禁用', 'Disabled', 'Nonaktif')}
                  </span>
                  {item.is_default ? (
                    <span style={{ color: '#F0B90B' }}>{tr('默认', 'Default', 'Default')}</span>
                  ) : null}
                </div>
              </div>
            ))
          )}
        </div>
      </section>

      <section className="rounded-xl border p-4" style={{ background: '#11161E', borderColor: '#2B3139' }}>
        <h2 className="mb-4 flex items-center gap-2 text-lg font-semibold" style={{ color: '#EAECEF' }}>
          <Shield className="h-5 w-5" />
          {tr('会员套餐配置', 'Membership Plan Config', 'Konfigurasi Paket Membership')}
        </h2>

        <div className="space-y-4">
          {sortedPlans.map((plan) => (
            <div key={plan.code} className="rounded border p-4" style={{ borderColor: '#2B3139' }}>
              <div className="mb-3 flex items-center justify-between">
                <div>
                  <div className="text-base font-semibold" style={{ color: '#EAECEF' }}>{plan.code}</div>
                  <div className="text-xs" style={{ color: '#848E9C' }}>{tr('价格按分存储，前端展示美元', 'Price stored in cents, displayed as USD', 'Harga disimpan sen, ditampilkan sebagai USD')}</div>
                </div>
                <button
                  type="button"
                  onClick={() => void savePlan(plan)}
                  disabled={savingPlanCode === plan.code}
                  className="flex items-center gap-2 rounded border px-3 py-2 text-sm disabled:opacity-60"
                  style={{ borderColor: '#F0B90B', color: '#F0B90B' }}
                >
                  <Save className="h-4 w-4" />
                  {savingPlanCode === plan.code
                    ? tr('保存中...', 'Saving...', 'Menyimpan...')
                    : tr('保存套餐', 'Save Plan', 'Simpan Paket')}
                </button>
              </div>

              <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                <input
                  value={plan.name}
                  onChange={(e) => updatePlan(plan.code, 'name', e.target.value)}
                  placeholder={tr('名称', 'Name', 'Nama')}
                  className="rounded border px-3 py-2"
                  style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                />
                <input
                  value={plan.currency}
                  onChange={(e) => updatePlan(plan.code, 'currency', e.target.value.toUpperCase())}
                  placeholder="USD"
                  className="rounded border px-3 py-2"
                  style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                />
                <select
                  value={plan.billing_cycle}
                  onChange={(e) => updatePlan(plan.code, 'billing_cycle', e.target.value)}
                  className="rounded border bg-transparent px-3 py-2"
                  style={{ borderColor: '#2B3139', color: '#EAECEF' }}
                >
                  <option value="monthly">monthly</option>
                  <option value="weekly">weekly</option>
                  <option value="yearly">yearly</option>
                </select>

                <input
                  type="number"
                  step="0.01"
                  value={centsToDisplay(plan.price_cents)}
                  onChange={(e) => updatePlan(plan.code, 'price_cents', displayToCents(e.target.value))}
                  placeholder="99.00"
                  className="rounded border px-3 py-2"
                  style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                />
                <input
                  type="number"
                  step="1"
                  value={plan.revenue_share_bps}
                  onChange={(e) => updatePlan(plan.code, 'revenue_share_bps', Number(e.target.value) || 0)}
                  placeholder="2000"
                  className="rounded border px-3 py-2"
                  style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                />
                <input
                  type="number"
                  step="1"
                  value={plan.seat_limit}
                  onChange={(e) => updatePlan(plan.code, 'seat_limit', Number(e.target.value) || 0)}
                  placeholder="0"
                  className="rounded border px-3 py-2"
                  style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                />

                <input
                  type="number"
                  step="1"
                  value={plan.sort_order}
                  onChange={(e) => updatePlan(plan.code, 'sort_order', Number(e.target.value) || 0)}
                  placeholder="10"
                  className="rounded border px-3 py-2"
                  style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                />
                <label className="flex items-center gap-2 text-sm" style={{ color: '#AEB4BC' }}>
                  <input
                    type="checkbox"
                    checked={plan.enabled}
                    onChange={(e) => updatePlan(plan.code, 'enabled', e.target.checked)}
                  />
                  {tr('启用套餐', 'Plan Enabled', 'Paket Aktif')}
                </label>
                <div />

                <textarea
                  value={plan.description}
                  onChange={(e) => updatePlan(plan.code, 'description', e.target.value)}
                  placeholder={tr('描述', 'Description', 'Deskripsi')}
                  className="rounded border px-3 py-2 md:col-span-3"
                  rows={2}
                  style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                />

                <textarea
                  value={plan.entitlements}
                  onChange={(e) => updatePlan(plan.code, 'entitlements', e.target.value)}
                  placeholder='{"signals":true,"managed_api":false}'
                  className="rounded border px-3 py-2 md:col-span-3"
                  rows={4}
                  style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF', fontFamily: 'monospace' }}
                />
              </div>
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}

