import { useEffect, useMemo, useState } from 'react'
import useSWR from 'swr'
import HeaderBar from '../components/HeaderBar'
import LoginModal from '../components/landing/LoginModal'
import { LoginRequiredOverlay } from '../components/LoginRequiredOverlay'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import { api } from '../lib/api'
import { notify } from '../lib/notify'
import type { MembershipPlanItem, ShowcaseWallCardItem } from '../types'
import { formatBacktestDays } from '../utils/backtestRun'

const fallbackWallCards: ShowcaseWallCardItem[] = [
  { strategy_id: 'fallback-1', strategy_name: 'Trend Follow', total_return_pct: 18.5, max_drawdown_pct: 12.4, win_rate: 62.0 },
  { strategy_id: 'fallback-2', strategy_name: 'Volatility Compression', total_return_pct: 11.8, max_drawdown_pct: 9.6, win_rate: 58.1 },
  { strategy_id: 'fallback-3', strategy_name: 'Momentum Breakout', total_return_pct: 9.4, max_drawdown_pct: 7.8, win_rate: 54.6 },
]

const fallbackMembershipPlans: MembershipPlanItem[] = [
  {
    code: 'bronze',
    name: 'Bronze',
    description: 'Signal following and backtest insights',
    price_cents: 9900,
    currency: 'USD',
    billing_cycle: 'monthly',
    revenue_share_bps: 0,
    seat_limit: 0,
    enabled: true,
    sort_order: 10,
    entitlements: '{}',
  },
  {
    code: 'diamond',
    name: 'Diamond',
    description: 'API auto trading with advanced risk control and operations support',
    price_cents: 49900,
    currency: 'USD',
    billing_cycle: 'monthly',
    revenue_share_bps: 2000,
    seat_limit: 50,
    enabled: true,
    sort_order: 20,
    entitlements: '{}',
  },
]

const formatPlanAmount = (priceCents: number, currency: string): string => {
  const amount = (Math.max(0, Number(priceCents) || 0) / 100).toFixed(2)
  return `${String(currency || 'USD').toUpperCase()} ${amount}`
}

const formatBillingCycleLabel = (cycle: string, isZh: boolean): string => {
  const value = String(cycle || '').trim().toLowerCase()
  if (value === 'yearly') return isZh ? '\u5e74\u4ed8' : 'yearly'
  if (value === 'weekly') return isZh ? '\u5468\u4ed8' : 'weekly'
  if (value === 'daily') return isZh ? '\u65e5\u4ed8' : 'daily'
  return isZh ? '\u6708\u4ed8' : 'monthly'
}

const buildEquityPolyline = (points: ShowcaseWallCardItem['equity_preview'] | undefined): string => {
  const data = Array.isArray(points) ? points : []
  if (data.length < 2) {
    return '10,170 70,162 130,152 190,142 250,128 310,114 370,100 430,86 490,74 550,62 610,50'
  }
  const min = Math.min(...data.map((p) => p.equity))
  const max = Math.max(...data.map((p) => p.equity))
  const span = Math.max(1e-9, max - min)
  const width = 620
  const left = 10
  const top = 20
  const height = 150
  return data
    .map((p, idx) => {
      const x = left + (idx / Math.max(1, data.length - 1)) * (width - left * 2)
      const y = top + ((max - p.equity) / span) * height
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
}

const goTo = (path: string) => {
  window.history.pushState({}, '', path)
  window.dispatchEvent(new PopStateEvent('popstate'))
}

export function LandingPage() {
  const [showLoginModal, setShowLoginModal] = useState(false)
  const [loginOverlayOpen, setLoginOverlayOpen] = useState(false)
  const [loginOverlayFeature, setLoginOverlayFeature] = useState('')
  const [payingPlanCode, setPayingPlanCode] = useState('')
  const [selectedPlanCode, setSelectedPlanCode] = useState('')
  const { user, logout } = useAuth()
  const { language, setLanguage } = useLanguage()
  const isZh = language === 'zh'
  const isLoggedIn = !!user

  const { data: wallCardsData } = useSWR('public-showcase-wall', api.getPublicBacktestShowcaseWall, {
    refreshInterval: 60000,
  })
  const wallCards = useMemo(() => {
    const items = (wallCardsData || []).filter((item) => String(item.strategy_id || '').trim() !== '')
    return items.length > 0 ? items : fallbackWallCards
  }, [wallCardsData])

  const { data: membershipPlansData } = useSWR('public-membership-plans', api.getMembershipPlans, {
    refreshInterval: 60000,
  })
  const membershipPlans = useMemo(() => {
    const items = (membershipPlansData || [])
      .filter((item) => item.enabled)
      .sort((a, b) => a.sort_order - b.sort_order || a.price_cents - b.price_cents)
    return items.length > 0 ? items : fallbackMembershipPlans
  }, [membershipPlansData])

  useEffect(() => {
    if (membershipPlans.length === 0) {
      setSelectedPlanCode('')
      return
    }
    if (!selectedPlanCode || !membershipPlans.some((plan) => plan.code === selectedPlanCode)) {
      setSelectedPlanCode(membershipPlans[0].code)
    }
  }, [membershipPlans, selectedPlanCode])

  const handleLoginRequired = (featureName: string) => {
    setLoginOverlayFeature(featureName)
    setLoginOverlayOpen(true)
  }

  const handleWallCardClick = (card: ShowcaseWallCardItem) => {
    const runID = String(card.run_id || card.showcase_run_id || '').trim()
    if (!runID) return
    const target = `/backtest?run_id=${encodeURIComponent(runID)}`
    if (!isLoggedIn) {
      sessionStorage.setItem('returnUrl', target)
      setShowLoginModal(true)
      return
    }
    goTo(target)
  }

  const handlePlanPurchase = async (plan: MembershipPlanItem) => {
    if (payingPlanCode) return
    if (!isLoggedIn) {
      sessionStorage.setItem('returnUrl', '/')
      setShowLoginModal(true)
      return
    }

    setPayingPlanCode(plan.code)
    try {
      const origin = window.location.origin
      const successURL = `${origin}/dashboard?payment=success&plan=${encodeURIComponent(plan.code)}`
      const failureURL = `${origin}/?payment=failed&plan=${encodeURIComponent(plan.code)}`

      const result = await api.createInfiniMembershipOrder({
        plan_code: plan.code,
        success_url: successURL,
        failure_url: failureURL,
        order_desc: `${plan.name || plan.code} membership`,
      })

      const checkoutURL = String(result.checkout_url || '').trim()
      if (!checkoutURL) {
        throw new Error('No checkout URL returned')
      }
      window.location.href = checkoutURL
    } catch (error) {
      notify.error(error instanceof Error ? error.message : 'Failed to start payment, please try again')
    } finally {
      setPayingPlanCode('')
    }
  }

  return (
    <>
      <HeaderBar
        onLoginClick={() => setShowLoginModal(true)}
        isLoggedIn={isLoggedIn}
        isHomePage={true}
        language={language}
        onLanguageChange={setLanguage}
        user={user}
        onLogout={logout}
        onLoginRequired={handleLoginRequired}
        onPageChange={(page) => {
          const pathMap: Record<string, string> = {
            data: '/data',
            competition: '/competition',
            'strategy-market': '/strategy-market',
            traders: '/traders',
            trader: '/dashboard',
            backtest: '/backtest',
            strategy: '/strategy',
            'strategy-permissions': '/strategy-permissions',
            'strategy-webhooks': '/strategy-webhooks',
            'payment-config': '/payment-config',
            debate: '/debate',
            faq: '/faq',
          }
          const path = pathMap[page]
          if (path) goTo(path)
        }}
      />

      <div className="min-h-screen bg-[#070A12] text-[#F5F7FA] pt-16">
        <section
          className="relative overflow-hidden py-12 md:py-16"
          style={{
            background:
              'radial-gradient(110% 130% at 80% -10%, rgba(57,117,255,0.24) 0%, rgba(7,10,18,1) 55%), radial-gradient(80% 100% at 5% 95%, rgba(16,185,129,0.18) 0%, rgba(7,10,18,0.95) 60%)',
          }}
        >
          <div className="mx-auto w-full max-w-6xl px-6 md:px-10">
            <p className="text-xs uppercase tracking-[0.25em] text-[#8AB4FF]">NewMoneyClub</p>
            <h1 className="mt-4 text-4xl font-bold leading-tight md:text-6xl">
              {isZh ? '\u522b\u518d\u76ef\u76d8\u4e86\uff0c\u8ba9 AI \u63a5\u7ba1\u4f60\u7684\u52a0\u5bc6\u4ea4\u6613\u3002' : 'Stop screen-watching. Let AI run your crypto trading.'}
            </h1>
            <p className="mt-5 max-w-3xl text-base md:text-xl text-[#C5D2E1]">
              {isZh
                ? '\u56de\u6d4b\u5148\u884c\uff0c\u7ecf\u9a8c\u53ef\u89c6\u5316\uff0c\u81ea\u52a8\u5316\u6267\u884c\u3002\u5148\u770b\u7ed3\u679c\uff0c\u518d\u51b3\u5b9a\u662f\u5426\u8ddf\u5355\u3002'
                : 'Backtest-first strategy showcase plus automated execution. Validate first, then decide.'}
            </p>
          </div>
          <div className="pointer-events-none absolute inset-0 opacity-20">
            <div className="absolute left-[-10%] top-12 h-64 w-64 rounded-full bg-[#4F8DFF] blur-3xl" />
            <div className="absolute right-[-12%] bottom-0 h-72 w-72 rounded-full bg-[#10B981] blur-3xl" />
          </div>
        </section>

        <section id="showcase-section" className="mx-auto w-full max-w-6xl px-6 py-12 md:px-10">
          <h2 className="text-2xl font-bold md:text-3xl">{isZh ? '\u5b98\u65b9\u7b56\u7565\u6a71\u7a97' : 'Official Strategy Showcase'}</h2>
          <div className="mt-2 text-sm text-[#9BB1C9]">
            {isZh ? '\u70b9\u51fb\u5361\u7247\u53ef\u8df3\u8f6c\u5bf9\u5e94\u56de\u6d4b\u3002' : 'Click a card to open its linked backtest run.'}
          </div>
          <div className="mt-6 grid gap-5 md:grid-cols-3">
            {wallCards.map((card) => {
              const runID = String(card.run_id || card.showcase_run_id || '').trim()
              const clickable = runID.length > 0
              return (
                <article
                  key={`${card.strategy_id}-${runID || 'no-run'}`}
                  onClick={() => clickable && handleWallCardClick(card)}
                  className="rounded-xl border border-[#263248] bg-[#0D1321] p-4 shadow-[0_0_30px_rgba(0,0,0,0.25)]"
                  style={{ cursor: clickable ? 'pointer' : 'default' }}
                >
                  <div className="mb-3 h-32 w-full rounded-lg border border-[#1F2A3D] bg-[#08101A] p-2">
                    <svg viewBox="0 0 620 190" className="h-full w-full" preserveAspectRatio="none">
                      <polyline fill="none" stroke="#10B981" strokeWidth="3" points={buildEquityPolyline(card.equity_preview)} />
                    </svg>
                  </div>
                  <h3 className="text-lg font-semibold truncate">{String(card.strategy_name || card.strategy_id || 'Strategy')}</h3>
                  <p className="mt-2 text-sm font-semibold text-[#F0B90B]">
                    Return {Number(card.total_return_pct || 0).toFixed(2)}% | Max DD {Number(card.max_drawdown_pct || 0).toFixed(2)}% | Win {Number(card.win_rate || 0).toFixed(1)}% | Days {formatBacktestDays(card.start_ts, card.end_ts)}
                  </p>
                  <p className="mt-2 text-sm text-[#AFC1D4] truncate">
                    {clickable ? `Open backtest: ${runID}` : 'No showcase run configured yet'}
                  </p>
                </article>
              )
            })}
          </div>
        </section>

        <section className="mx-auto w-full max-w-6xl px-6 py-4 md:px-10">
          <h2 className="text-2xl font-bold md:text-3xl">Why Backtest First</h2>
          <div className="mt-5 overflow-hidden rounded-xl border border-[#263248]">
            <table className="w-full text-sm">
              <thead className="bg-[#0D1321] text-left">
                <tr>
                  <th className="px-4 py-3">Item</th>
                  <th className="px-4 py-3">Manual Trading</th>
                  <th className="px-4 py-3">NewMoneyClub</th>
                </tr>
              </thead>
              <tbody className="bg-[#0A111D] text-[#D3DFEB]">
                <tr className="border-t border-[#1F2A3D]">
                  <td className="px-4 py-3">Decision Basis</td>
                  <td className="px-4 py-3">Experience and emotion</td>
                  <td className="px-4 py-3">Historical validation + strategy rules + AI execution</td>
                </tr>
                <tr className="border-t border-[#1F2A3D]">
                  <td className="px-4 py-3">Discipline</td>
                  <td className="px-4 py-3">Easily affected by volatility</td>
                  <td className="px-4 py-3">Consistent risk control and rule execution</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section className="mx-auto w-full max-w-6xl px-6 pb-16 pt-10 md:px-10">
          <h2 className="text-2xl font-bold md:text-3xl">{isZh ? '\u6536\u8d39\u5957\u9910' : 'Plans'}</h2>
          <div className="mt-2 text-sm text-[#9BB1C9]">
            {isZh ? '\u4ee5\u4e0b\u5957\u9910\u6765\u81ea\u4f1a\u5458\u5957\u9910\u914d\u7f6e\uff0c\u53ef\u52a8\u6001\u7ef4\u62a4\u3002' : 'Plans below are loaded from membership plan configuration.'}
          </div>
          {selectedPlanCode ? (
            <div className="mt-2 text-xs font-semibold text-[#F0B90B]">
              {isZh ? `\u5f53\u524d\u9009\u62e9: ${selectedPlanCode.toUpperCase()}` : `Selected: ${selectedPlanCode.toUpperCase()}`}
            </div>
          ) : null}
          <div className="mt-6 grid gap-5 md:grid-cols-2 lg:grid-cols-3">
            {membershipPlans.map((plan) => {
              const highlight = plan.revenue_share_bps > 0 || plan.seat_limit > 0
              const isPaying = payingPlanCode === plan.code
              const isSelected = selectedPlanCode === plan.code
              return (
                <article
                  key={plan.code}
                  onClick={() => setSelectedPlanCode(plan.code)}
                  className={
                    isSelected
                      ? 'rounded-xl border-2 border-[#F0B90B] bg-[#162235] p-6 shadow-[0_0_30px_rgba(240,185,11,0.2)] transition hover:-translate-y-0.5'
                      : highlight
                        ? 'rounded-xl border border-[#F0B90B] bg-[#131B2B] p-6 shadow-[0_0_30px_rgba(240,185,11,0.15)] transition hover:-translate-y-0.5'
                        : 'rounded-xl border border-[#263248] bg-[#0D1321] p-6 transition hover:-translate-y-0.5'
                  }
                  style={{ cursor: isPaying ? 'wait' : 'pointer' }}
                >
                  {isSelected && (
                    <div className="mb-2 inline-flex rounded-full bg-[#F0B90B] px-3 py-1 text-xs font-bold text-black">
                      {isZh ? '\u5df2\u9009\u4e2d' : 'Selected'}
                    </div>
                  )}
                  {plan.seat_limit > 0 && (
                    <div className="mb-2 inline-flex rounded-full bg-[#F0B90B] px-3 py-1 text-xs font-bold text-black">
                      {isZh ? `\u9650\u91cf ${plan.seat_limit} \u540d` : `Limited ${plan.seat_limit} seats`}
                    </div>
                  )}
                  <h3 className="text-xl font-semibold">{plan.name || plan.code}</h3>
                  <p className="mt-3 text-3xl font-bold text-[#F0B90B]">
                    {formatPlanAmount(plan.price_cents, plan.currency)} / {formatBillingCycleLabel(plan.billing_cycle, isZh)}
                    {plan.revenue_share_bps > 0 && ` + ${(plan.revenue_share_bps / 100).toFixed(2)}% rev share`}
                  </p>
                  <p className="mt-3 text-sm text-[#B8CADC]">
                    {plan.description || 'Please set a description in payment config.'}
                  </p>
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation()
                      if (!isSelected) {
                        setSelectedPlanCode(plan.code)
                        return
                      }
                      void handlePlanPurchase(plan)
                    }}
                    disabled={!!payingPlanCode}
                    className={
                      isSelected
                        ? 'mt-5 rounded-lg bg-[#F0B90B] px-4 py-2 text-sm font-bold text-black hover:bg-[#e0ad09] disabled:cursor-not-allowed disabled:opacity-70'
                        : 'mt-5 rounded-lg border border-[#F0B90B] bg-transparent px-4 py-2 text-sm font-bold text-[#F0B90B] hover:bg-[#F0B90B]/10 disabled:cursor-not-allowed disabled:opacity-70'
                    }
                  >
                    {isPaying ? 'Redirecting...' : isSelected ? (isZh ? '\u7acb\u5373\u5f00\u901a' : 'Subscribe Now') : (isZh ? '\u9009\u62e9\u5957\u9910' : 'Select Plan')}
                  </button>
                </article>
              )
            })}
          </div>
        </section>

        {showLoginModal && <LoginModal onClose={() => setShowLoginModal(false)} language={language} />}

        <LoginRequiredOverlay
          isOpen={loginOverlayOpen}
          onClose={() => setLoginOverlayOpen(false)}
          featureName={loginOverlayFeature}
        />
      </div>
    </>
  )
}
