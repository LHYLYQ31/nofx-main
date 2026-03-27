export function getPremiumStrategyName(rawName: string): string {
  const name = String(rawName || '').trim()
  if (!name) return ''

  if (/(网格|grid)/i.test(name)) {
    if (/(多币|multi)/i.test(name)) {
      return '多币种动态再平衡 (Dynamic Rebalancing)'
    }
    return '流动性套利 (Liquidity Arbitrage)'
  }

  return name
}

export function getBlackboxModelName(): string {
  return 'NewMoney 宏观情绪主脑 (Powered by M3 Ultra & Gemini Pro)'
}
