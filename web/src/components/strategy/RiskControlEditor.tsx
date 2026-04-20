import { Shield, AlertTriangle } from 'lucide-react'
import type { RiskControlConfig } from '../../types'

interface RiskControlEditorProps {
  config: RiskControlConfig
  onChange: (config: RiskControlConfig) => void
  disabled?: boolean
  language: string
}

export function RiskControlEditor({
  config,
  onChange,
  disabled,
  language,
}: RiskControlEditorProps) {
  const t = (key: string) => {
    const translations: Record<string, Record<string, string>> = {
      positionLimits: { zh: '仓位限制', en: 'Position Limits' },
      maxPositions: { zh: '最大持仓数量', en: 'Max Positions' },
      maxPositionsDesc: { zh: '同时持有的最大币种数量', en: 'Maximum coins held simultaneously' },
      // Trading leverage (exchange leverage)
      tradingLeverage: { zh: '交易杠杆（交易所杠杆）', en: 'Trading Leverage (Exchange)' },
      btcEthLeverage: { zh: 'BTC/ETH 交易杠杆', en: 'BTC/ETH Trading Leverage' },
      btcEthLeverageDesc: { zh: '交易所开仓使用的杠杆倍数', en: 'Exchange leverage for opening positions' },
      altcoinLeverage: { zh: '山寨币交易杠杆', en: 'Altcoin Trading Leverage' },
      altcoinLeverageDesc: { zh: '交易所开仓使用的杠杆倍数', en: 'Exchange leverage for opening positions' },
      // Position value ratio (risk control) - CODE ENFORCED
      positionValueRatio: { zh: '仓位价值比例（代码强制）', en: 'Position Value Ratio (CODE ENFORCED)' },
      positionValueRatioDesc: { zh: '单仓位名义价值 / 账户净值，由代码强制执行', en: 'Position notional value / equity, enforced by code' },
      btcEthPositionValueRatio: { zh: 'BTC/ETH 仓位价值比例', en: 'BTC/ETH Position Value Ratio' },
      btcEthPositionValueRatioDesc: { zh: '单仓最大名义价值 = 净值 × 此值（代码强制）', en: 'Max position value = equity × this ratio (CODE ENFORCED)' },
      altcoinPositionValueRatio: { zh: '山寨币仓位价值比例', en: 'Altcoin Position Value Ratio' },
      altcoinPositionValueRatioDesc: { zh: '单仓最大名义价值 = 净值 × 此值（代码强制）', en: 'Max position value = equity × this ratio (CODE ENFORCED)' },
      riskParameters: { zh: '风险参数', en: 'Risk Parameters' },
      minRiskReward: { zh: '最小风险回报比', en: 'Min Risk/Reward Ratio' },
      minRiskRewardDesc: { zh: '开仓要求的最低盈亏比', en: 'Minimum profit ratio for opening' },
      maxMarginUsage: { zh: '最大保证金使用率（代码强制）', en: 'Max Margin Usage (CODE ENFORCED)' },
      maxMarginUsageDesc: { zh: '保证金使用率上限，由代码强制执行', en: 'Maximum margin utilization, enforced by code' },
      entryRequirements: { zh: '开仓要求', en: 'Entry Requirements' },
      minPositionSize: { zh: '最小开仓金额', en: 'Min Position Size' },
      minPositionSizeDesc: { zh: 'USDT 最小名义价值', en: 'Minimum notional value in USDT' },
      minConfidence: { zh: '最小信心度', en: 'Min Confidence' },
      minConfidenceDesc: { zh: 'AI 开仓信心度阈值', en: 'AI confidence threshold for entry' },
      atrVolatilityStop: { zh: 'ATR Volatility Stop', en: 'ATR Volatility Stop' },
      atrVolatilityStopDesc: { zh: 'Code-enforced adaptive stop-loss based on ATR', en: 'Code-enforced adaptive stop-loss based on ATR' },
      atrStopEnabled: { zh: 'Enable ATR Stop', en: 'Enable ATR Stop' },
      atrStopMultiplier: { zh: 'ATR Multiplier', en: 'ATR Multiplier' },
      atrStopMultiplierDesc: { zh: 'Stop distance = ATR(14) x multiplier', en: 'Stop distance = ATR(14) x multiplier' },
      executionGuards: { zh: '执行防护（通用）', en: 'Execution Guards (Generic)' },
      executionGuardsDesc: { zh: '跨策略通用执行风控：价差门限、成交后二次RR、SL/TP失败降级', en: 'Strategy-agnostic execution risk guards' },
      priceDeviationLimitPct: { zh: '价差门限(%)', en: 'Price Deviation Limit (%)' },
      priceDeviationLimitPctDesc: { zh: '市场价与 AI entry_price 偏差超过该值时，跳过开仓', en: 'Skip entry when market deviates too far from AI entry_price' },
      postFillRRRecheckEnabled: { zh: '启用成交后二次RR校验', en: 'Enable Post-Fill RR Recheck' },
      postFillRRTolerance: { zh: 'RR容忍度', en: 'RR Tolerance' },
      postFillRRToleranceDesc: { zh: '有效阈值 = 最小RR - 容忍度', en: 'Effective threshold = Min RR - tolerance' },
      postFillRROnFail: { zh: '二次RR失败处理', en: 'On Post-Fill RR Fail' },
      sltpRetryCount: { zh: 'SL/TP 重试次数', en: 'SL/TP Retry Count' },
      sltpRetryIntervalMs: { zh: 'SL/TP 重试间隔(ms)', en: 'SL/TP Retry Interval (ms)' },
      onSLFail: { zh: '止损挂单失败处理', en: 'On SL Placement Fail' },
      onTPFail: { zh: '止盈挂单失败处理', en: 'On TP Placement Fail' },
      actionAdjustTP: { zh: '自动调整止盈', en: 'Adjust TP' },
      actionCloseImmediately: { zh: '立即平仓', en: 'Close Immediately' },
      actionAlertOnly: { zh: '仅告警', en: 'Alert Only' },
      actionKeepWithSLRetry: { zh: '保留仓位并继续补挂', en: 'Keep Position and Retry TP' },
    }
    return translations[key]?.[language] || key
  }

  const updateField = <K extends keyof RiskControlConfig>(
    key: K,
    value: RiskControlConfig[K]
  ) => {
    if (!disabled) {
      onChange({ ...config, [key]: value })
    }
  }

  return (
    <div className="space-y-6">
      {/* Position Limits */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Shield className="w-5 h-5" style={{ color: '#F0B90B' }} />
          <h3 className="font-medium" style={{ color: '#EAECEF' }}>
            {t('positionLimits')}
          </h3>
        </div>

        <div className="grid grid-cols-1 gap-4 mb-4">
          <div
            className="p-4 rounded-lg"
            style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
          >
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('maxPositions')}
            </label>
            <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
              {t('maxPositionsDesc')}
            </p>
            <input
              type="number"
              value={config.max_positions ?? 3}
              onChange={(e) =>
                updateField('max_positions', parseInt(e.target.value) || 3)
              }
              disabled={disabled}
              min={1}
              max={10}
              className="w-32 px-3 py-2 rounded"
              style={{
                background: '#1E2329',
                border: '1px solid #2B3139',
                color: '#EAECEF',
              }}
            />
          </div>
        </div>

        {/* Trading Leverage (Exchange) */}
        <div className="mb-2">
          <p className="text-xs font-medium mb-2" style={{ color: '#F0B90B' }}>
            {t('tradingLeverage')}
          </p>
        </div>
        <div className="grid grid-cols-2 gap-4 mb-4">
          <div
            className="p-4 rounded-lg"
            style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
          >
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('btcEthLeverage')}
            </label>
            <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
              {t('btcEthLeverageDesc')}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={config.btc_eth_max_leverage ?? 5}
                onChange={(e) =>
                  updateField('btc_eth_max_leverage', parseInt(e.target.value))
                }
                disabled={disabled}
                min={1}
                max={20}
                className="flex-1 accent-yellow-500"
              />
              <span
                className="w-12 text-center font-mono"
                style={{ color: '#F0B90B' }}
              >
                {config.btc_eth_max_leverage ?? 5}x
              </span>
            </div>
          </div>

          <div
            className="p-4 rounded-lg"
            style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
          >
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('altcoinLeverage')}
            </label>
            <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
              {t('altcoinLeverageDesc')}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={config.altcoin_max_leverage ?? 5}
                onChange={(e) =>
                  updateField('altcoin_max_leverage', parseInt(e.target.value))
                }
                disabled={disabled}
                min={1}
                max={20}
                className="flex-1 accent-yellow-500"
              />
              <span
                className="w-12 text-center font-mono"
                style={{ color: '#F0B90B' }}
              >
                {config.altcoin_max_leverage ?? 5}x
              </span>
            </div>
          </div>
        </div>

        {/* Position Value Ratio (Risk Control - CODE ENFORCED) */}
        <div className="mb-2">
          <p className="text-xs font-medium" style={{ color: '#0ECB81' }}>
            {t('positionValueRatio')}
          </p>
          <p className="text-xs mt-1" style={{ color: '#848E9C' }}>
            {t('positionValueRatioDesc')}
          </p>
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div
            className="p-4 rounded-lg"
            style={{ background: '#0B0E11', border: '1px solid #0ECB81' }}
          >
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('btcEthPositionValueRatio')}
            </label>
            <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
              {t('btcEthPositionValueRatioDesc')}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={config.btc_eth_max_position_value_ratio ?? 5}
                onChange={(e) =>
                  updateField('btc_eth_max_position_value_ratio', parseFloat(e.target.value))
                }
                disabled={disabled}
                min={0.5}
                max={10}
                step={0.5}
                className="flex-1 accent-green-500"
              />
              <span
                className="w-12 text-center font-mono"
                style={{ color: '#0ECB81' }}
              >
                {config.btc_eth_max_position_value_ratio ?? 5}x
              </span>
            </div>
          </div>

          <div
            className="p-4 rounded-lg"
            style={{ background: '#0B0E11', border: '1px solid #0ECB81' }}
          >
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('altcoinPositionValueRatio')}
            </label>
            <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
              {t('altcoinPositionValueRatioDesc')}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={config.altcoin_max_position_value_ratio ?? 1}
                onChange={(e) =>
                  updateField('altcoin_max_position_value_ratio', parseFloat(e.target.value))
                }
                disabled={disabled}
                min={0.5}
                max={10}
                step={0.5}
                className="flex-1 accent-green-500"
              />
              <span
                className="w-12 text-center font-mono"
                style={{ color: '#0ECB81' }}
              >
                {config.altcoin_max_position_value_ratio ?? 1}x
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Risk Parameters */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <AlertTriangle className="w-5 h-5" style={{ color: '#F6465D' }} />
          <h3 className="font-medium" style={{ color: '#EAECEF' }}>
            {t('riskParameters')}
          </h3>
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div
            className="p-4 rounded-lg"
            style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
          >
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('minRiskReward')}
            </label>
            <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
              {t('minRiskRewardDesc')}
            </p>
            <div className="flex items-center">
              <span style={{ color: '#848E9C' }}>1:</span>
              <input
                type="number"
                value={config.min_risk_reward_ratio ?? 3}
                onChange={(e) =>
                  updateField('min_risk_reward_ratio', parseFloat(e.target.value) || 3)
                }
                disabled={disabled}
                min={1}
                max={10}
                step={0.5}
                className="w-20 px-3 py-2 rounded ml-2"
                style={{
                  background: '#1E2329',
                  border: '1px solid #2B3139',
                  color: '#EAECEF',
                }}
              />
            </div>
          </div>

          <div
            className="p-4 rounded-lg"
            style={{ background: '#0B0E11', border: '1px solid #0ECB81' }}
          >
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('maxMarginUsage')}
            </label>
            <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
              {t('maxMarginUsageDesc')}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={(config.max_margin_usage ?? 0.9) * 100}
                onChange={(e) =>
                  updateField('max_margin_usage', parseInt(e.target.value) / 100)
                }
                disabled={disabled}
                min={10}
                max={100}
                className="flex-1 accent-green-500"
              />
              <span className="w-12 text-center font-mono" style={{ color: '#0ECB81' }}>
                {Math.round((config.max_margin_usage ?? 0.9) * 100)}%
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* ATR Volatility Stop */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <AlertTriangle className="w-5 h-5" style={{ color: '#F0B90B' }} />
          <h3 className="font-medium" style={{ color: '#EAECEF' }}>
            {t('atrVolatilityStop')}
          </h3>
        </div>

        <div
          className="p-4 rounded-lg"
          style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
        >
          <p className="text-xs mb-3" style={{ color: '#848E9C' }}>
            {t('atrVolatilityStopDesc')}
          </p>

          <label className="flex items-center gap-2 mb-4 cursor-pointer">
            <input
              type="checkbox"
              checked={config.atr_stop_enabled ?? false}
              onChange={(e) => updateField('atr_stop_enabled', e.target.checked)}
              disabled={disabled}
              className="accent-yellow-500"
            />
            <span className="text-sm" style={{ color: '#EAECEF' }}>
              {t('atrStopEnabled')}
            </span>
          </label>

          <div className="flex items-center gap-3">
            <div className="flex-1">
              <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
                {t('atrStopMultiplier')}
              </label>
              <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
                {t('atrStopMultiplierDesc')}
              </p>
              <input
                type="range"
                value={config.atr_stop_multiplier ?? 1.5}
                onChange={(e) =>
                  updateField('atr_stop_multiplier', parseFloat(e.target.value) || 1.5)
                }
                disabled={disabled || !(config.atr_stop_enabled ?? false)}
                min={0.5}
                max={5}
                step={0.1}
                className="w-full accent-yellow-500"
              />
            </div>
            <input
              type="number"
              value={config.atr_stop_multiplier ?? 1.5}
              onChange={(e) =>
                updateField('atr_stop_multiplier', parseFloat(e.target.value) || 1.5)
              }
              disabled={disabled || !(config.atr_stop_enabled ?? false)}
              min={0.5}
              max={5}
              step={0.1}
              className="w-24 px-3 py-2 rounded"
              style={{
                background: '#1E2329',
                border: '1px solid #2B3139',
                color: '#EAECEF',
              }}
            />
          </div>
        </div>
      </div>

      {/* Execution Guards */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <AlertTriangle className="w-5 h-5" style={{ color: '#F0B90B' }} />
          <h3 className="font-medium" style={{ color: '#EAECEF' }}>
            {t('executionGuards')}
          </h3>
        </div>
        <p className="text-xs mb-4" style={{ color: '#848E9C' }}>
          {t('executionGuardsDesc')}
        </p>

        <div className="grid grid-cols-2 gap-4">
          <div className="p-4 rounded-lg" style={{ background: '#0B0E11', border: '1px solid #2B3139' }}>
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('priceDeviationLimitPct')}
            </label>
            <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
              {t('priceDeviationLimitPctDesc')}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="number"
                value={config.price_deviation_limit_pct ?? 0.35}
                onChange={(e) => updateField('price_deviation_limit_pct', parseFloat(e.target.value) || 0.35)}
                disabled={disabled}
                min={0}
                max={5}
                step={0.05}
                className="w-24 px-3 py-2 rounded"
                style={{ background: '#1E2329', border: '1px solid #2B3139', color: '#EAECEF' }}
              />
              <span style={{ color: '#848E9C' }}>%</span>
            </div>
          </div>

          <div className="p-4 rounded-lg" style={{ background: '#0B0E11', border: '1px solid #2B3139' }}>
            <label className="flex items-center gap-2 mb-3 cursor-pointer">
              <input
                type="checkbox"
                checked={config.post_fill_rr_recheck_enabled ?? true}
                onChange={(e) => updateField('post_fill_rr_recheck_enabled', e.target.checked)}
                disabled={disabled}
                className="accent-yellow-500"
              />
              <span className="text-sm" style={{ color: '#EAECEF' }}>
                {t('postFillRRRecheckEnabled')}
              </span>
            </label>
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('postFillRRTolerance')}
            </label>
            <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
              {t('postFillRRToleranceDesc')}
            </p>
            <input
              type="number"
              value={config.post_fill_rr_tolerance ?? 0.1}
              onChange={(e) => updateField('post_fill_rr_tolerance', parseFloat(e.target.value) || 0.1)}
              disabled={disabled || !(config.post_fill_rr_recheck_enabled ?? true)}
              min={0}
              max={2}
              step={0.05}
              className="w-24 px-3 py-2 rounded"
              style={{ background: '#1E2329', border: '1px solid #2B3139', color: '#EAECEF' }}
            />
          </div>

          <div className="p-4 rounded-lg" style={{ background: '#0B0E11', border: '1px solid #2B3139' }}>
            <label className="block text-sm mb-2" style={{ color: '#EAECEF' }}>
              {t('postFillRROnFail')}
            </label>
            <select
              value={config.post_fill_rr_on_fail ?? 'adjust_tp'}
              onChange={(e) => updateField('post_fill_rr_on_fail', e.target.value)}
              disabled={disabled || !(config.post_fill_rr_recheck_enabled ?? true)}
              className="w-full px-3 py-2 rounded"
              style={{ background: '#1E2329', border: '1px solid #2B3139', color: '#EAECEF' }}
            >
              <option value="adjust_tp">{t('actionAdjustTP')}</option>
              <option value="close_immediately">{t('actionCloseImmediately')}</option>
              <option value="alert_only">{t('actionAlertOnly')}</option>
            </select>
          </div>

          <div className="p-4 rounded-lg" style={{ background: '#0B0E11', border: '1px solid #2B3139' }}>
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('sltpRetryCount')}
            </label>
            <input
              type="number"
              value={config.sltp_retry_count ?? 3}
              onChange={(e) => updateField('sltp_retry_count', parseInt(e.target.value) || 3)}
              disabled={disabled}
              min={0}
              max={10}
              step={1}
              className="w-24 px-3 py-2 rounded mb-3"
              style={{ background: '#1E2329', border: '1px solid #2B3139', color: '#EAECEF' }}
            />
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('sltpRetryIntervalMs')}
            </label>
            <input
              type="number"
              value={config.sltp_retry_interval_ms ?? 1000}
              onChange={(e) => updateField('sltp_retry_interval_ms', parseInt(e.target.value) || 1000)}
              disabled={disabled}
              min={0}
              max={10000}
              step={100}
              className="w-32 px-3 py-2 rounded"
              style={{ background: '#1E2329', border: '1px solid #2B3139', color: '#EAECEF' }}
            />
          </div>

          <div className="p-4 rounded-lg" style={{ background: '#0B0E11', border: '1px solid #2B3139' }}>
            <label className="block text-sm mb-2" style={{ color: '#EAECEF' }}>
              {t('onSLFail')}
            </label>
            <select
              value={config.on_sl_fail ?? 'close_immediately'}
              onChange={(e) => updateField('on_sl_fail', e.target.value)}
              disabled={disabled}
              className="w-full px-3 py-2 rounded"
              style={{ background: '#1E2329', border: '1px solid #2B3139', color: '#EAECEF' }}
            >
              <option value="close_immediately">{t('actionCloseImmediately')}</option>
              <option value="alert_only">{t('actionAlertOnly')}</option>
            </select>
          </div>

          <div className="p-4 rounded-lg" style={{ background: '#0B0E11', border: '1px solid #2B3139' }}>
            <label className="block text-sm mb-2" style={{ color: '#EAECEF' }}>
              {t('onTPFail')}
            </label>
            <select
              value={config.on_tp_fail ?? 'keep_with_sl_and_retry'}
              onChange={(e) => updateField('on_tp_fail', e.target.value)}
              disabled={disabled}
              className="w-full px-3 py-2 rounded"
              style={{ background: '#1E2329', border: '1px solid #2B3139', color: '#EAECEF' }}
            >
              <option value="keep_with_sl_and_retry">{t('actionKeepWithSLRetry')}</option>
              <option value="close_immediately">{t('actionCloseImmediately')}</option>
              <option value="alert_only">{t('actionAlertOnly')}</option>
            </select>
          </div>
        </div>
      </div>

      {/* Entry Requirements */}
      <div>
        <div className="flex items-center gap-2 mb-4">
          <Shield className="w-5 h-5" style={{ color: '#0ECB81' }} />
          <h3 className="font-medium" style={{ color: '#EAECEF' }}>
            {t('entryRequirements')}
          </h3>
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div
            className="p-4 rounded-lg"
            style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
          >
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('minPositionSize')}
            </label>
            <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
              {t('minPositionSizeDesc')}
            </p>
            <div className="flex items-center">
              <input
                type="number"
                value={config.min_position_size ?? 12}
                onChange={(e) =>
                  updateField('min_position_size', parseFloat(e.target.value) || 12)
                }
                disabled={disabled}
                min={10}
                max={1000}
                className="w-24 px-3 py-2 rounded"
                style={{
                  background: '#1E2329',
                  border: '1px solid #2B3139',
                  color: '#EAECEF',
                }}
              />
              <span className="ml-2" style={{ color: '#848E9C' }}>
                USDT
              </span>
            </div>
          </div>

          <div
            className="p-4 rounded-lg"
            style={{ background: '#0B0E11', border: '1px solid #2B3139' }}
          >
            <label className="block text-sm mb-1" style={{ color: '#EAECEF' }}>
              {t('minConfidence')}
            </label>
            <p className="text-xs mb-2" style={{ color: '#848E9C' }}>
              {t('minConfidenceDesc')}
            </p>
            <div className="flex items-center gap-2">
              <input
                type="range"
                value={config.min_confidence ?? 75}
                onChange={(e) =>
                  updateField('min_confidence', parseInt(e.target.value))
                }
                disabled={disabled}
                min={50}
                max={100}
                className="flex-1 accent-green-500"
              />
              <span className="w-12 text-center font-mono" style={{ color: '#0ECB81' }}>
                {config.min_confidence ?? 75}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
