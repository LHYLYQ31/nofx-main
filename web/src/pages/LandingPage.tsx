import { useMemo, useState } from 'react'
import useSWR from 'swr'
import HeaderBar from '../components/HeaderBar'
import LoginModal from '../components/landing/LoginModal'
import { LoginRequiredOverlay } from '../components/LoginRequiredOverlay'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import { api } from '../lib/api'
import type { ShowcaseWallCardItem } from '../types'
import { formatBacktestDays } from '../utils/backtestRun'

const fallbackWallCards: ShowcaseWallCardItem[] = [
  { strategy_id: 'fallback-1', strategy_name: '趋势跟随', total_return_pct: 18.5, max_drawdown_pct: 12.4, win_rate: 62.0 },
  { strategy_id: 'fallback-2', strategy_name: '波动挤压', total_return_pct: 11.8, max_drawdown_pct: 9.6, win_rate: 58.1 },
  { strategy_id: 'fallback-3', strategy_name: '突破动量', total_return_pct: 9.4, max_drawdown_pct: 7.8, win_rate: 54.6 },
]

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
              {isZh ? '别再盯盘了，让 AI 接管你的加密交易。' : 'Stop screen-watching. Let AI run your crypto trading.'}
            </h1>
            <p className="mt-5 max-w-3xl text-base md:text-xl text-[#C5D2E1]">
              {isZh
                ? '策略回测可视化 + 实盘自动执行。先看验证结果，再决定是否跟随。'
                : 'Backtest-first strategy showcase plus automated execution. Validate first, then decide.'}
            </p>
            <div className="mt-7 flex flex-wrap gap-3">
              <button
                onClick={() => document.getElementById('showcase-section')?.scrollIntoView({ behavior: 'smooth' })}
                className="rounded-lg bg-[#F0B90B] px-6 py-3 text-sm font-bold text-black hover:bg-[#e0ad09] transition"
              >
                {isZh ? '立即查看策略橱窗' : 'View Strategy Showcase'}
              </button>
              <button
                onClick={() => goTo('/backtest')}
                className="rounded-lg border border-[#36506E] bg-[#0B1320] px-6 py-3 text-sm font-semibold text-[#D9E6F2] hover:border-[#4C6E95]"
              >
                {isZh ? '进入回测中心' : 'Open Backtest Lab'}
              </button>
            </div>
          </div>
          <div className="pointer-events-none absolute inset-0 opacity-20">
            <div className="absolute left-[-10%] top-12 h-64 w-64 rounded-full bg-[#4F8DFF] blur-3xl" />
            <div className="absolute right-[-12%] bottom-0 h-72 w-72 rounded-full bg-[#10B981] blur-3xl" />
          </div>
        </section>

        <section id="showcase-section" className="mx-auto w-full max-w-6xl px-6 py-12 md:px-10">
          <h2 className="text-2xl font-bold md:text-3xl">{isZh ? '官方授权策略橱窗' : 'Official Strategy Showcase'}</h2>
          <div className="mt-2 text-sm text-[#9BB1C9]">
            {isZh ? '点击卡片查看对应回测详情；未登录会先引导登录。' : 'Click a card to open its linked backtest run.'}
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
                  <h3 className="text-lg font-semibold truncate">
                    {String(card.strategy_name || card.strategy_id || 'Strategy')}
                  </h3>
                  <p className="mt-2 text-sm font-semibold text-[#F0B90B]">
                    {isZh ? '总收益率' : 'Return'} {Number(card.total_return_pct || 0).toFixed(2)}% |{' '}
                    {isZh ? '最大回撤' : 'Max DD'} {Number(card.max_drawdown_pct || 0).toFixed(2)}% |{' '}
                    {isZh ? '胜率' : 'Win'} {Number(card.win_rate || 0).toFixed(1)}% |{' '}
                    {isZh ? '回测天数' : 'Days'} {formatBacktestDays(card.start_ts, card.end_ts)}
                  </p>
                  <p className="mt-2 text-sm text-[#AFC1D4] truncate">
                    {clickable
                      ? isZh ? `点击查看回测：${runID}` : `Open backtest: ${runID}`
                      : isZh ? '暂未绑定展示回测 run' : 'No showcase run configured yet'}
                  </p>
                </article>
              )
            })}
          </div>
        </section>

        <section className="mx-auto w-full max-w-6xl px-6 py-4 md:px-10">
          <h2 className="text-2xl font-bold md:text-3xl">{isZh ? '为什么先看回测' : 'Why Backtest First'}</h2>
          <div className="mt-5 overflow-hidden rounded-xl border border-[#263248]">
            <table className="w-full text-sm">
              <thead className="bg-[#0D1321] text-left">
                <tr>
                  <th className="px-4 py-3">{isZh ? '对比项' : 'Item'}</th>
                  <th className="px-4 py-3">{isZh ? '主观交易' : 'Manual Trading'}</th>
                  <th className="px-4 py-3">NewMoneyClub</th>
                </tr>
              </thead>
              <tbody className="bg-[#0A111D] text-[#D3DFEB]">
                <tr className="border-t border-[#1F2A3D]">
                  <td className="px-4 py-3">{isZh ? '决策依据' : 'Decision Basis'}</td>
                  <td className="px-4 py-3">{isZh ? '经验和情绪' : 'Experience and emotion'}</td>
                  <td className="px-4 py-3">{isZh ? '历史验证 + 策略规则 + AI 执行' : 'Historical validation + strategy rules + AI execution'}</td>
                </tr>
                <tr className="border-t border-[#1F2A3D]">
                  <td className="px-4 py-3">{isZh ? '交易纪律' : 'Discipline'}</td>
                  <td className="px-4 py-3">{isZh ? '容易受波动影响' : 'Easily affected by volatility'}</td>
                  <td className="px-4 py-3">{isZh ? '统一风控，规则执行' : 'Consistent risk control and rule execution'}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section className="mx-auto w-full max-w-6xl px-6 pb-16 pt-10 md:px-10">
          <h2 className="text-2xl font-bold md:text-3xl">{isZh ? '收费门槛' : 'Plans'}</h2>
          <div className="mt-6 grid gap-5 md:grid-cols-2">
            <article className="rounded-xl border border-[#263248] bg-[#0D1321] p-6">
              <h3 className="text-xl font-semibold">{isZh ? '青铜会员 / 信号跟单' : 'Bronze / Signal Follow'}</h3>
              <p className="mt-3 text-3xl font-bold text-[#F0B90B]">$99/月</p>
              <p className="mt-3 text-sm text-[#B8CADC]">
                {isZh ? '每日策略信号与回测更新，适合轻量跟单和策略观察。' : 'Daily strategy signals and backtest updates.'}
              </p>
            </article>
            <article className="rounded-xl border border-[#F0B90B] bg-[#131B2B] p-6 shadow-[0_0_30px_rgba(240,185,11,0.15)]">
              <div className="mb-2 inline-flex rounded-full bg-[#F0B90B] px-3 py-1 text-xs font-bold text-black">
                {isZh ? '限量内测名额' : 'Limited Beta Slots'}
              </div>
              <h3 className="text-xl font-semibold">{isZh ? '钻石会员 / API 全自动托管' : 'Diamond / API Auto Trading'}</h3>
              <p className="mt-3 text-3xl font-bold text-[#F0B90B]">$499/月 + 分成</p>
              <p className="mt-3 text-sm text-[#B8CADC]">
                {isZh ? '系统直连交易账户，自动执行策略，提供高级风控与运营支持。' : 'Fully automated strategy execution with advanced risk controls.'}
              </p>
            </article>
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
