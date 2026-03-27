import { useState } from 'react'
import HeaderBar from '../components/HeaderBar'
import LoginModal from '../components/landing/LoginModal'
import { LoginRequiredOverlay } from '../components/LoginRequiredOverlay'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'

const strategyCards = [
  {
    title: '大周期趋势跟随者',
    metrics: '历史年化 +185% | 最大回撤仅 12.4% | 胜率 62%',
    desc: '滤除震荡，专吃单边。AI 智能追踪大户建仓点。',
    equity:
      '10,180 70,165 130,150 190,128 250,112 310,96 370,83 430,70 490,55 550,42 610,30',
  },
  {
    title: 'SMC 聪明钱猎手',
    metrics: '单笔极高盈亏比 1:4 | 捕捉流动性真空',
    desc: '高敏捷多空切换，优先打击机构流动性缺口。',
    equity:
      '10,176 70,170 130,158 190,146 250,131 310,114 370,98 430,80 490,63 550,46 610,34',
  },
  {
    title: '极端情绪反转（高频）',
    metrics: '日均开单 3 次 | 胜率 75% | 专治震荡市',
    desc: '情绪过热即反身交易，快速获利后严格止盈止损。',
    equity:
      '10,175 70,160 130,154 190,138 250,123 310,108 370,92 430,79 490,65 550,49 610,36',
  },
]

export function LandingPage() {
  const [showLoginModal, setShowLoginModal] = useState(false)
  const [loginOverlayOpen, setLoginOverlayOpen] = useState(false)
  const [loginOverlayFeature, setLoginOverlayFeature] = useState('')
  const { user, logout } = useAuth()
  const { language, setLanguage } = useLanguage()
  const isLoggedIn = !!user

  const handleLoginRequired = (featureName: string) => {
    setLoginOverlayFeature(featureName)
    setLoginOverlayOpen(true)
  }

  const scrollToShowcase = () => {
    document.getElementById('showcase-section')?.scrollIntoView({ behavior: 'smooth' })
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
            debate: '/debate',
            faq: '/faq',
          }
          const path = pathMap[page]
          if (path) window.location.href = path
        }}
      />

      <div className="min-h-screen bg-[#070A12] text-[#F5F7FA] pt-16">
        <section
          className="relative px-6 py-20 md:px-10 md:py-28 overflow-hidden"
          style={{
            background:
              'radial-gradient(110% 120% at 80% 0%, rgba(57,117,255,0.22) 0%, rgba(7,10,18,1) 50%), radial-gradient(90% 90% at 10% 90%, rgba(16,185,129,0.14) 0%, rgba(7,10,18,0.95) 55%)',
          }}
        >
          <div className="mx-auto max-w-6xl relative z-10">
            <p className="text-xs uppercase tracking-[0.25em] text-[#8AB4FF]">NewMoney Club</p>
            <h1 className="mt-4 text-4xl font-bold leading-tight md:text-6xl">
              别再盯盘了。让硅谷的 AI，接管你的加密资产。
            </h1>
            <p className="mt-5 max-w-3xl text-base md:text-xl text-[#C5D2E1]">
              基于大语言模型的情绪感知 + 华尔街毫秒级量化执行。全自动运行，无需下载软件。
            </p>
            <button
              onClick={scrollToShowcase}
              className="mt-8 rounded-lg bg-[#F0B90B] px-6 py-3 text-sm font-bold text-black hover:bg-[#e0ad09] transition"
            >
              立即查看实盘战绩
            </button>
          </div>
          <div className="pointer-events-none absolute inset-0 opacity-20">
            <div className="absolute left-[-10%] top-12 h-64 w-64 rounded-full bg-[#4F8DFF] blur-3xl" />
            <div className="absolute right-[-12%] bottom-0 h-72 w-72 rounded-full bg-[#10B981] blur-3xl" />
          </div>
        </section>

        <section id="showcase-section" className="mx-auto max-w-6xl px-6 py-16 md:px-10">
          <h2 className="text-2xl font-bold md:text-3xl">三大“神级策略”橱窗</h2>
          <div className="mt-8 grid gap-5 md:grid-cols-3">
            {strategyCards.map((card) => (
              <article
                key={card.title}
                className="rounded-xl border border-[#263248] bg-[#0D1321] p-4 shadow-[0_0_30px_rgba(0,0,0,0.25)]"
              >
                <div className="mb-3 h-32 w-full rounded-lg border border-[#1F2A3D] bg-[#08101A] p-2">
                  <svg viewBox="0 0 620 190" className="h-full w-full" preserveAspectRatio="none">
                    <polyline fill="none" stroke="#10B981" strokeWidth="3" points={card.equity} />
                  </svg>
                </div>
                <h3 className="text-lg font-semibold">{card.title}</h3>
                <p className="mt-2 text-sm font-semibold text-[#F0B90B]">{card.metrics}</p>
                <p className="mt-2 text-sm text-[#AFC1D4]">{card.desc}</p>
              </article>
            ))}
          </div>
        </section>

        <section className="mx-auto max-w-6xl px-6 py-16 md:px-10">
          <h2 className="text-2xl font-bold md:text-3xl">黑盒降维打击</h2>
          <div className="mt-6 overflow-hidden rounded-xl border border-[#263248]">
            <table className="w-full text-sm">
              <thead className="bg-[#0D1321] text-left">
                <tr>
                  <th className="px-4 py-3">对比项</th>
                  <th className="px-4 py-3">传统买指标散户</th>
                  <th className="px-4 py-3">NewMoney 俱乐部会员</th>
                </tr>
              </thead>
              <tbody className="bg-[#0A111D] text-[#D3DFEB]">
                <tr className="border-t border-[#1F2A3D]">
                  <td className="px-4 py-3">执行方式</td>
                  <td className="px-4 py-3">需要自己盯盘 ❌</td>
                  <td className="px-4 py-3">AI 7x24 读新闻、毫秒级跟单 ✅</td>
                </tr>
                <tr className="border-t border-[#1F2A3D]">
                  <td className="px-4 py-3">下单延迟</td>
                  <td className="px-4 py-3">自己下单，滑点高 ❌</td>
                  <td className="px-4 py-3">云端服务器零延迟 ✅</td>
                </tr>
                <tr className="border-t border-[#1F2A3D]">
                  <td className="px-4 py-3">风险处置</td>
                  <td className="px-4 py-3">黑天鹅死扛爆仓 ❌</td>
                  <td className="px-4 py-3">严格移动止损 ✅</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section className="mx-auto max-w-6xl px-6 pb-20 md:px-10">
          <h2 className="text-2xl font-bold md:text-3xl">收费门槛</h2>
          <div className="mt-7 grid gap-5 md:grid-cols-2">
            <article className="rounded-xl border border-[#263248] bg-[#0D1321] p-6">
              <h3 className="text-xl font-semibold">青铜会员 / Discord 喊单</h3>
              <p className="mt-3 text-3xl font-bold text-[#F0B90B]">$99/月</p>
              <p className="mt-3 text-sm text-[#B8CADC]">包含每日核心信号推送，适合资金小的体验用户。</p>
            </article>
            <article className="rounded-xl border border-[#F0B90B] bg-[#131B2B] p-6 shadow-[0_0_30px_rgba(240,185,11,0.15)]">
              <div className="mb-2 inline-flex rounded-full bg-[#F0B90B] px-3 py-1 text-xs font-bold text-black">
                🔥 仅剩 15 个内测名额
              </div>
              <h3 className="text-xl font-semibold">钻石会员 / API 云端全自动托管</h3>
              <p className="mt-3 text-3xl font-bold text-[#F0B90B]">$499/月 + 盈利分红</p>
              <p className="mt-3 text-sm text-[#B8CADC]">系统直连你的 Hyperliquid，睡后收入。</p>
            </article>
          </div>
        </section>

        {showLoginModal && (
          <LoginModal
            onClose={() => setShowLoginModal(false)}
            language={language}
          />
        )}

        <LoginRequiredOverlay
          isOpen={loginOverlayOpen}
          onClose={() => setLoginOverlayOpen(false)}
          featureName={loginOverlayFeature}
        />
      </div>
    </>
  )
}
