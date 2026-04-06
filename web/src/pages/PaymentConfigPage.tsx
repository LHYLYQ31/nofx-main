import { useEffect, useMemo, useState } from 'react'
import { CreditCard, Plus, RefreshCw, Save, Shield } from 'lucide-react'
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

function fieldLabel(label: string) {
  return (
    <div className="mb-1 text-xs" style={{ color: '#848E9C' }}>
      {label}
    </div>
  )
}

export function PaymentConfigPage() {
  const { language } = useLanguage()
  const tr = (zh: string, en: string) => (language === 'zh' ? zh : en)

  const [loading, setLoading] = useState(true)
  const [savingProvider, setSavingProvider] = useState(false)
  const [savingPlanCode, setSavingPlanCode] = useState<string>('')
  const [newPlanCode, setNewPlanCode] = useState('')

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
          : tr('加载支付配置失败', 'Failed to load payment settings')
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
      notify.error(tr('Base URL 和商户公钥（Key ID）必填', 'Base URL and merchant public key (Key ID) are required'))
      return
    }

    if (!providerForm.id && !providerForm.secret_key.trim()) {
      notify.error(tr('首次创建必须填写商户私钥（Secret Key）', 'Merchant private key (Secret Key) is required for new config'))
      return
    }

    if (!providerForm.id && !providerForm.webhook_secret.trim()) {
      notify.error(tr('首次创建必须填写 Webhook Secret', 'Webhook Secret is required for new config'))
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
      notify.success(tr('支付配置已保存', 'Payment provider config saved'))
      await loadAll()
    } catch (error) {
      notify.error(
        error instanceof Error
          ? error.message
          : tr('保存支付配置失败', 'Failed to save payment provider config')
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

  const addPlan = () => {
    const code = newPlanCode.trim().toLowerCase()
    if (!code) {
      notify.error(tr('请输入套餐编码（例如 silver）', 'Please enter a plan code (e.g. silver)'))
      return
    }
    if (!/^[a-z0-9_-]+$/.test(code)) {
      notify.error(tr('套餐编码仅支持小写字母、数字、-、_', 'Plan code only supports lowercase letters, numbers, - and _'))
      return
    }
    if (plans.some((plan) => plan.code === code)) {
      notify.error(tr('该套餐编码已存在', 'This plan code already exists'))
      return
    }

    const maxSort = plans.reduce((max, item) => Math.max(max, item.sort_order), 0)
    const created: MembershipPlanItem = {
      code,
      name: code,
      description: '',
      price_cents: 0,
      currency: 'USD',
      billing_cycle: 'monthly',
      revenue_share_bps: 0,
      seat_limit: 0,
      enabled: true,
      sort_order: maxSort + 10,
      entitlements: '{}',
    }
    setPlans((prev) => [created, ...prev])
    setNewPlanCode('')
    notify.success(tr(`已新增套餐 ${code}，请填写参数后点击“保存套餐”`, `Plan ${code} added. Fill fields and click "Save Plan"`))
  }

  const savePlan = async (plan: MembershipPlanItem) => {
    if (!plan.name.trim()) {
      notify.error(tr('套餐名称不能为空', 'Plan name is required'))
      return
    }

    try {
      JSON.parse(plan.entitlements || '{}')
    } catch {
      notify.error(
        tr(
          `套餐 ${plan.code} 的权益 JSON 格式错误`,
          `Entitlements JSON is invalid for plan ${plan.code}`
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
      notify.success(tr(`套餐 ${plan.code} 已保存`, `Membership plan ${plan.code} saved`))
      await loadAll()
    } catch (error) {
      notify.error(
        error instanceof Error
          ? error.message
          : tr('保存会员套餐失败', 'Failed to save membership plan')
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
            {tr('支付参数配置（Infini）', 'Payment Provider Config (Infini)')}
          </h2>
          <button
            type="button"
            onClick={() => void loadAll()}
            disabled={loading}
            className="flex items-center gap-2 rounded border px-3 py-2 text-sm"
            style={{ borderColor: '#2B3139', color: '#AEB4BC' }}
          >
            <RefreshCw className="h-4 w-4" />
            {tr('刷新', 'Refresh')}
          </button>
        </div>
        {loading ? (
          <div className="mb-3 text-xs" style={{ color: '#848E9C' }}>
            {tr('正在加载配置...', 'Loading settings...')}
          </div>
        ) : null}

        <div className="mb-4 grid grid-cols-1 gap-3 md:grid-cols-3">
          <div>
            {fieldLabel(tr('配置版本', 'Config Version'))}
            <select
              value={selectedProviderConfigID}
              onChange={(e) => onSelectProvider(e.target.value)}
              className="w-full rounded border bg-transparent px-3 py-2"
              style={{ borderColor: '#2B3139', color: '#EAECEF' }}
            >
              <option value="">{tr('新建配置', 'New Config')}</option>
              {providerConfigs.map((item) => (
                <option key={item.id} value={item.id}>
                  {`${item.display_name || item.environment} v${item.version}${item.is_default ? ' (default)' : ''}`}
                </option>
              ))}
            </select>
          </div>

          <div>
            {fieldLabel(tr('显示名称', 'Display Name'))}
            <input
              value={providerForm.display_name}
              onChange={(e) => setProviderForm((prev) => ({ ...prev, display_name: e.target.value }))}
              placeholder={tr('例如：Infini Production', 'e.g. Infini Production')}
              className="w-full rounded border px-3 py-2"
              style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
            />
          </div>

          <div>
            {fieldLabel(tr('环境', 'Environment'))}
            <select
              value={providerForm.environment}
              onChange={(e) => setProviderForm((prev) => ({ ...prev, environment: e.target.value }))}
              className="w-full rounded border bg-transparent px-3 py-2"
              style={{ borderColor: '#2B3139', color: '#EAECEF' }}
            >
              <option value="production">production</option>
              <option value="sandbox">sandbox</option>
            </select>
          </div>

          <div className="md:col-span-2">
            {fieldLabel('Base URL')}
            <input
              value={providerForm.base_url}
              onChange={(e) => setProviderForm((prev) => ({ ...prev, base_url: e.target.value }))}
              placeholder="https://openapi.infini.money"
              className="w-full rounded border px-3 py-2"
              style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
            />
          </div>

          <div>
            {fieldLabel(tr('公钥 / Key ID', 'Public Key / Key ID'))}
            <input
              value={providerForm.key_id}
              onChange={(e) => setProviderForm((prev) => ({ ...prev, key_id: e.target.value }))}
              placeholder={tr('填写开发者页面的商户公钥（即 keyId）', 'Paste merchant public key from Developer page (this is keyId)')}
              className="w-full rounded border px-3 py-2"
              style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
            />
          </div>

          <div className="md:col-span-2">
            {fieldLabel(tr('私钥 / Secret Key', 'Private Key / Secret Key'))}
            <textarea
              value={providerForm.secret_key}
              onChange={(e) => setProviderForm((prev) => ({ ...prev, secret_key: e.target.value }))}
              placeholder={
                selectedProvider?.has_secret_key
                  ? tr('留空则保持现有私钥（Secret Key）', 'Leave blank to keep existing private key (Secret Key)')
                  : tr('填写开发者页面的商户私钥', 'Paste merchant private key from Developer page')
              }
              rows={3}
              className="w-full rounded border px-3 py-2"
              style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
            />
          </div>

          <div className="md:col-span-2">
            {fieldLabel('Webhook Secret')}
            <textarea
              value={providerForm.webhook_secret}
              onChange={(e) => setProviderForm((prev) => ({ ...prev, webhook_secret: e.target.value }))}
              placeholder={
                selectedProvider?.has_webhook_secret
                  ? tr('留空则保持现有 Webhook Secret', 'Leave blank to keep existing Webhook Secret')
                  : tr('填写开发者页面的 Webhook Secret', 'Paste Webhook Secret from Developer page')
              }
              rows={2}
              className="w-full rounded border px-3 py-2"
              style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
            />
          </div>

          <div className="flex items-end gap-4 pb-2 text-sm" style={{ color: '#AEB4BC' }}>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={providerForm.enabled}
                onChange={(e) => setProviderForm((prev) => ({ ...prev, enabled: e.target.checked }))}
              />
              {tr('启用', 'Enabled')}
            </label>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={providerForm.is_default}
                onChange={(e) => setProviderForm((prev) => ({ ...prev, is_default: e.target.checked }))}
              />
              {tr('设为默认', 'Set as default')}
            </label>
          </div>
        </div>

        <div className="mb-4 text-xs" style={{ color: '#848E9C' }}>
          {tr(
            '说明：支付参数存数据库。密钥会加密存储；下单时后端动态加载当前启用配置。',
            'Note: payment params are stored in DB. Secrets are encrypted; runtime loads active config dynamically.'
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
          {savingProvider ? tr('保存中...', 'Saving...') : tr('保存支付配置', 'Save Payment Config')}
        </button>
      </section>

      <section className="rounded-xl border p-4" style={{ background: '#11161E', borderColor: '#2B3139' }}>
        <h2 className="mb-4 flex items-center gap-2 text-lg font-semibold" style={{ color: '#EAECEF' }}>
          <Shield className="h-5 w-5" />
          {tr('会员套餐配置', 'Membership Plan Config')}
        </h2>

        <div className="mb-4 grid grid-cols-1 gap-3 md:grid-cols-[1fr_auto]">
          <input
            type="text"
            value={newPlanCode}
            onChange={(e) => setNewPlanCode(e.target.value)}
            placeholder={tr('新增套餐编码，例如：silver', 'New plan code, e.g. silver')}
            className="w-full rounded border px-3 py-2"
            style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
          />
          <button
            type="button"
            onClick={addPlan}
            className="flex items-center justify-center gap-2 rounded border px-3 py-2 text-sm"
            style={{ borderColor: '#F0B90B', color: '#F0B90B' }}
          >
            <Plus className="h-4 w-4" />
            {tr('新增套餐', 'Add Plan')}
          </button>
        </div>

        <div className="space-y-4">
          {sortedPlans.map((plan) => (
            <div key={plan.code} className="rounded border p-4" style={{ borderColor: '#2B3139' }}>
              <div className="mb-3 flex items-center justify-between">
                <div>
                  <div className="text-base font-semibold" style={{ color: '#EAECEF' }}>{plan.code}</div>
                  <div className="text-xs" style={{ color: '#848E9C' }}>
                    {tr('价格按 cents 存储，页面展示为美元。', 'Price stored in cents, displayed as USD.')}
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => void savePlan(plan)}
                  disabled={savingPlanCode === plan.code}
                  className="flex items-center gap-2 rounded border px-3 py-2 text-sm disabled:opacity-60"
                  style={{ borderColor: '#F0B90B', color: '#F0B90B' }}
                >
                  <Save className="h-4 w-4" />
                  {savingPlanCode === plan.code ? tr('保存中...', 'Saving...') : tr('保存套餐', 'Save Plan')}
                </button>
              </div>

              <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                <div>
                  {fieldLabel(tr('套餐名称（字符串）', 'Plan Name (text)'))}
                  <input
                    type="text"
                    value={plan.name}
                    onChange={(e) => updatePlan(plan.code, 'name', e.target.value)}
                    placeholder={tr('例如：Bronze', 'e.g. Bronze')}
                    className="w-full rounded border px-3 py-2"
                    style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                  />
                </div>

                <div>
                  {fieldLabel(tr('币种', 'Currency'))}
                  <input
                    type="text"
                    value={plan.currency}
                    onChange={(e) => updatePlan(plan.code, 'currency', e.target.value.toUpperCase())}
                    placeholder="USD"
                    className="w-full rounded border px-3 py-2"
                    style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                  />
                </div>

                <div>
                  {fieldLabel(tr('计费周期', 'Billing Cycle'))}
                  <select
                    value={plan.billing_cycle}
                    onChange={(e) => updatePlan(plan.code, 'billing_cycle', e.target.value)}
                    className="w-full rounded border bg-transparent px-3 py-2"
                    style={{ borderColor: '#2B3139', color: '#EAECEF' }}
                  >
                    <option value="monthly">monthly</option>
                    <option value="weekly">weekly</option>
                    <option value="yearly">yearly</option>
                  </select>
                </div>

                <div>
                  {fieldLabel(tr('价格（USD）', 'Price (USD)'))}
                  <input
                    type="number"
                    step="0.01"
                    value={centsToDisplay(plan.price_cents)}
                    onChange={(e) => updatePlan(plan.code, 'price_cents', displayToCents(e.target.value))}
                    placeholder="99.00"
                    className="w-full rounded border px-3 py-2"
                    style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                  />
                </div>

                <div>
                  {fieldLabel(tr('分成比例（bps）', 'Revenue Share (bps)'))}
                  <input
                    type="number"
                    step="1"
                    value={plan.revenue_share_bps}
                    onChange={(e) => updatePlan(plan.code, 'revenue_share_bps', Number(e.target.value) || 0)}
                    placeholder="2000"
                    className="w-full rounded border px-3 py-2"
                    style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                  />
                </div>

                <div>
                  {fieldLabel(tr('名额上限（0=不限）', 'Seat Limit (0 = unlimited)'))}
                  <input
                    type="number"
                    step="1"
                    value={plan.seat_limit}
                    onChange={(e) => updatePlan(plan.code, 'seat_limit', Number(e.target.value) || 0)}
                    placeholder="0"
                    className="w-full rounded border px-3 py-2"
                    style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                  />
                </div>

                <div>
                  {fieldLabel(tr('排序权重', 'Sort Order'))}
                  <input
                    type="number"
                    step="1"
                    value={plan.sort_order}
                    onChange={(e) => updatePlan(plan.code, 'sort_order', Number(e.target.value) || 0)}
                    placeholder="10"
                    className="w-full rounded border px-3 py-2"
                    style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                  />
                </div>

                <div className="flex items-end pb-2">
                  <label className="flex items-center gap-2 text-sm" style={{ color: '#AEB4BC' }}>
                    <input
                      type="checkbox"
                      checked={plan.enabled}
                      onChange={(e) => updatePlan(plan.code, 'enabled', e.target.checked)}
                    />
                    {tr('启用套餐', 'Plan Enabled')}
                  </label>
                </div>

                <div />

                <div className="md:col-span-3">
                  {fieldLabel(tr('套餐描述', 'Description'))}
                  <textarea
                    value={plan.description}
                    onChange={(e) => updatePlan(plan.code, 'description', e.target.value)}
                    placeholder={tr('例如：Signal following and backtest insights', 'e.g. Signal following and backtest insights')}
                    className="w-full rounded border px-3 py-2"
                    rows={2}
                    style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF' }}
                  />
                </div>

                <div className="md:col-span-3">
                  {fieldLabel(tr('权益 JSON（必须是合法 JSON）', 'Entitlements JSON (must be valid JSON)'))}
                  <textarea
                    value={plan.entitlements}
                    onChange={(e) => updatePlan(plan.code, 'entitlements', e.target.value)}
                    placeholder='{"signals":true,"managed_api":false}'
                    className="w-full rounded border px-3 py-2"
                    rows={4}
                    style={{ borderColor: '#2B3139', background: 'transparent', color: '#EAECEF', fontFamily: 'monospace' }}
                  />
                </div>
              </div>
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}
