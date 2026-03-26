const equityPoints = [
  '0,210',
  '70,180',
  '140,195',
  '210,130',
  '280,155',
  '350,120',
  '420,95',
  '490,110',
  '560,72',
  '630,40',
]

const benchmarkPoints = [
  '0,210',
  '70,200',
  '140,198',
  '210,194',
  '280,186',
  '350,180',
  '420,172',
  '490,166',
  '560,158',
  '630,150',
]

const strategyCards = [
  { name: 'Trend Pulse v2', pnl: '+184.2%', mdd: '-12.8%', win: '61%', period: '180D' },
  { name: 'Grid Reactor', pnl: '+96.4%', mdd: '-7.1%', win: '68%', period: '120D' },
  { name: 'Mean Revert X', pnl: '+72.9%', mdd: '-5.4%', win: '73%', period: '90D' },
]

export function BacktestHomePreviewPage() {
  return (
    <div className="min-h-screen bg-[#070A0F] text-[#E6EDF5]">
      <header className="sticky top-0 z-20 border-b border-white/10 bg-[#070A0F]/90 backdrop-blur">
        <div className="mx-auto flex h-16 w-full max-w-7xl items-center justify-between px-6">
          <div className="text-sm font-semibold tracking-[0.2em] text-[#EAB308]">
            NOFX PREVIEW
          </div>
          <a
            href="/"
            className="rounded-md border border-white/20 px-4 py-2 text-xs text-white/80 hover:border-[#EAB308]/50 hover:text-[#EAB308]"
          >
            Back to Current Home
          </a>
        </div>
      </header>

      <main className="mx-auto w-full max-w-7xl px-6 pb-24 pt-14">
        <section className="grid gap-10 lg:grid-cols-12">
          <div className="lg:col-span-5">
            <p className="mb-4 text-xs uppercase tracking-[0.3em] text-[#EAB308]">
              Backtest-first Home
            </p>
            <h1 className="text-4xl font-semibold leading-tight md:text-5xl">
              用可验证回测曲线，筛选真正可执行的策略
            </h1>
            <p className="mt-5 max-w-xl text-base text-white/70">
              静态预览版：首页主视觉改为策略净值图 + 回撤图 + 核心指标，先看展示风格，不接入真实接口。
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
              <button className="rounded-md bg-[#EAB308] px-5 py-3 text-sm font-semibold text-black">
                Start Backtest
              </button>
              <button className="rounded-md border border-white/20 px-5 py-3 text-sm font-semibold text-white/90">
                Open Strategy Market
              </button>
            </div>
          </div>

          <div className="rounded-2xl border border-white/10 bg-[#0B1018] p-5 shadow-[0_0_50px_rgba(0,0,0,0.35)] lg:col-span-7">
            <div className="mb-4 grid grid-cols-2 gap-3 md:grid-cols-5">
              {[
                ['累计收益', '+184.2%'],
                ['最大回撤', '-12.8%'],
                ['Sharpe', '2.14'],
                ['胜率', '61%'],
                ['交易次数', '486'],
              ].map(([k, v]) => (
                <div key={k} className="rounded-lg border border-white/10 bg-black/20 px-3 py-2">
                  <div className="text-[11px] text-white/60">{k}</div>
                  <div className="mt-1 text-base font-semibold text-[#EAB308]">{v}</div>
                </div>
              ))}
            </div>

            <div className="rounded-xl border border-white/10 bg-[#060A10] p-3">
              <svg viewBox="0 0 640 230" className="h-[230px] w-full">
                <defs>
                  <linearGradient id="eq" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#22C55E" stopOpacity="0.35" />
                    <stop offset="100%" stopColor="#22C55E" stopOpacity="0.02" />
                  </linearGradient>
                </defs>
                <g opacity="0.18" stroke="#ffffff">
                  <line x1="0" y1="40" x2="640" y2="40" />
                  <line x1="0" y1="90" x2="640" y2="90" />
                  <line x1="0" y1="140" x2="640" y2="140" />
                  <line x1="0" y1="190" x2="640" y2="190" />
                </g>
                <polyline
                  fill="none"
                  stroke="#94A3B8"
                  strokeWidth="2"
                  points={benchmarkPoints.join(' ')}
                />
                <polygon
                  fill="url(#eq)"
                  points={`0,230 ${equityPoints.join(' ')} 630,230`}
                />
                <polyline
                  fill="none"
                  stroke="#22C55E"
                  strokeWidth="3"
                  points={equityPoints.join(' ')}
                />
                <circle cx="560" cy="72" r="4" fill="#22C55E" />
                <circle cx="350" cy="120" r="4" fill="#22C55E" />
                <circle cx="210" cy="130" r="4" fill="#EF4444" />
                <circle cx="70" cy="180" r="4" fill="#EF4444" />
              </svg>
              <div className="mt-2 text-xs text-white/50">
                Strategy Equity (green) vs Benchmark (gray) | static preview mock data
              </div>
            </div>
          </div>
        </section>

        <section className="mt-12">
          <h2 className="mb-5 text-xl font-semibold">策略样例卡片</h2>
          <div className="grid gap-4 md:grid-cols-3">
            {strategyCards.map((card) => (
              <article
                key={card.name}
                className="rounded-xl border border-white/10 bg-[#0B1018] p-4 transition hover:border-[#EAB308]/40"
              >
                <h3 className="text-lg font-semibold">{card.name}</h3>
                <div className="mt-4 space-y-2 text-sm text-white/75">
                  <div className="flex justify-between">
                    <span>PnL</span>
                    <span className="font-semibold text-[#22C55E]">{card.pnl}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>MDD</span>
                    <span className="font-semibold text-[#F97316]">{card.mdd}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>Win Rate</span>
                    <span className="font-semibold">{card.win}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>Period</span>
                    <span className="font-semibold">{card.period}</span>
                  </div>
                </div>
              </article>
            ))}
          </div>
        </section>
      </main>
    </div>
  )
}
