import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import useSWR from 'swr'
import {
  Activity,
  AlertTriangle,
  CandlestickChart as CandlestickIcon,
  Clock,
  LineChart,
  Medal,
  RefreshCw,
  Search,
  TrendingUp,
} from 'lucide-react'
import {
  CandlestickSeries,
  ColorType,
  CrosshairMode,
  createChart,
  createSeriesMarkers,
  type CandlestickData,
  type IChartApi,
  type ISeriesApi,
  type SeriesMarker,
  type UTCTimestamp,
} from 'lightweight-charts'
import {
  Area,
  AreaChart,
  CartesianGrid,
  ReferenceDot,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { api } from '../lib/api'
import { useLanguage } from '../contexts/LanguageContext'
import { getPremiumStrategyName } from '../utils/displayName'
import type {
  BacktestEquityPoint,
  BacktestKline,
  BacktestKlinesResponse,
  BacktestMetrics,
  BacktestRunMetadata,
  BacktestTradeEvent,
  DecisionAction,
  DecisionRecord,
  ShowcaseStrategyItem,
} from '../types'

type StateFilter = 'all' | 'running' | 'paused' | 'done' | 'error'

const stateColor: Record<string, string> = {
  running: '#0ECB81',
  paused: '#F0B90B',
  done: '#14B8A6',
  error: '#F6465D',
  stopped: '#848E9C',
}

export function BacktestShowcasePage() {
  const { language } = useLanguage()
  const isZh = language === 'zh'

  const [search, setSearch] = useState('')
  const [stateFilter, setStateFilter] = useState<StateFilter>('all')
  const [selectedRunId, setSelectedRunId] = useState('')

  const query = useMemo(
    () => ({
      state: stateFilter === 'all' ? undefined : stateFilter,
      search: search.trim() || undefined,
      limit: 100,
      offset: 0,
    }),
    [search, stateFilter]
  )

  const { data: runsResp, isLoading: runsLoading, mutate: refreshRuns } = useSWR(
    ['showcase-backtest-runs', query],
    () => api.getBacktestShowcaseRuns(query),
    { refreshInterval: 30000 }
  )
  const { data: strategyWallConfig } = useSWR<ShowcaseStrategyItem[]>(
    'showcase-strategy-wall',
    api.getBacktestShowcaseStrategies,
    { refreshInterval: 60000 }
  )

  const runs = runsResp?.items ?? []
  const showcaseRuns = useMemo(
    () => runs.filter((r) => (r.summary?.equity_last ?? 0) >= 1000),
    [runs]
  )
  const selectedRun =
    showcaseRuns.find((r) => r.run_id === selectedRunId) ?? showcaseRuns[0]

  useEffect(() => {
    if (!showcaseRuns.length) {
      setSelectedRunId('')
      return
    }
    if (
      !selectedRunId ||
      !showcaseRuns.some((r) => r.run_id === selectedRunId)
    ) {
      setSelectedRunId(showcaseRuns[0].run_id)
    }
  }, [showcaseRuns, selectedRunId])

  const { data: metrics } = useSWR<BacktestMetrics>(
    selectedRun ? ['showcase-backtest-metrics', selectedRun.run_id] : null,
    () => api.getBacktestMetrics(selectedRun!.run_id)
  )

  const { data: equity } = useSWR<BacktestEquityPoint[]>(
    selectedRun ? ['showcase-backtest-equity', selectedRun.run_id] : null,
    () => api.getBacktestEquity(selectedRun!.run_id, '1m', 2000)
  )

  const { data: trades } = useSWR<BacktestTradeEvent[]>(
    selectedRun ? ['showcase-backtest-trades', selectedRun.run_id] : null,
    () => api.getBacktestTrades(selectedRun!.run_id, 200)
  )

  const { data: decisions } = useSWR<DecisionRecord[]>(
    selectedRun ? ['showcase-backtest-decisions', selectedRun.run_id] : null,
    () => api.getBacktestDecisions(selectedRun!.run_id, 10, 0)
  )

  const wallCards = useMemo(() => {
    const cfg = strategyWallConfig ?? []
    return cfg.map((item, idx) => {
      const matchedRun = showcaseRuns.find((r) => r.strategy_id === item.strategy_id)
      return {
        key: item.strategy_id || String(idx),
        name: getPremiumStrategyName(item.strategy_name || `Strategy ${idx + 1}`),
        runId: matchedRun?.run_id || '',
        symbol: matchedRun?.symbols?.[0] || '',
        pnl: matchedRun ? Math.max(0, Number((matchedRun.summary?.equity_last ?? 0) / 1000)) : 0,
        dd: matchedRun?.summary?.max_drawdown_pct ?? 0,
      }
    })
  }, [strategyWallConfig, showcaseRuns])

  const title = isZh ? '策略回测橱窗' : 'Strategy Backtest Showcase'
  const subtitle = isZh
    ? '展示可订阅策略的历史回测表现，帮助你筛选和购买更合适的策略。'
    : 'Explore historical backtest performance of subscribable strategies to choose what to buy.'

  return (
    <div className="min-h-screen" style={{ background: '#0B0E11', color: '#EAECEF' }}>
      <div className="mx-auto w-full max-w-[1440px] px-3 sm:px-4 py-5 md:py-6 md:px-6">
        <section
          className="rounded-2xl p-5 md:p-6"
          style={{
            background:
              'radial-gradient(100% 130% at 0% 0%, rgba(240,185,11,0.18) 0%, rgba(14,19,27,0.95) 45%, rgba(11,14,17,1) 100%)',
            border: '1px solid rgba(240,185,11,0.25)',
          }}
        >
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <h1 className="mt-1 text-2xl font-semibold md:text-3xl">{title}</h1>
              <p className="mt-2 text-sm md:text-base" style={{ color: '#AEB4BC' }}>
                {subtitle}
              </p>
            </div>
            <button
              onClick={() => void refreshRuns()}
              className="inline-flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-semibold"
              style={{ background: '#1E2329', border: '1px solid #2B3139', color: '#EAECEF' }}
            >
              <RefreshCw size={14} />
              {isZh ? '刷新' : 'Refresh'}
            </button>
          </div>
        </section>

        <section className="mt-5 overflow-hidden rounded-xl p-4" style={{ background: '#11151B', border: '1px solid #2B3139' }}>
          <div className="mb-3 text-sm font-semibold" style={{ color: '#EAECEF' }}>
            {isZh ? '官方策略照片墙' : 'Official Strategy Wall'}
          </div>
          {wallCards.length === 0 ? (
            <div className="py-8 text-sm" style={{ color: '#848E9C' }}>
              {isZh
                ? '暂未配置照片墙策略，请到策略授权页面配置。'
                : 'No strategy wall config yet. Configure it on Strategy Permission page.'}
            </div>
          ) : wallCards.length <= 4 ? (
            <div
              className="wall-grid gap-3"
              style={{ ['--wall-cols' as string]: String(Math.max(1, wallCards.length)) }}
            >
              {wallCards.map((card, idx) => (
                <article
                  key={card.key}
                  onClick={() => card.runId && setSelectedRunId(card.runId)}
                  className="overflow-hidden rounded-xl transition-all"
                  style={{
                    background: '#0B0E11',
                    border: '1px solid #2B3139',
                    cursor: card.runId ? 'pointer' : 'default',
                  }}
                >
                  <MiniKlinePanel
                    runId={card.runId}
                    symbol={card.symbol}
                    coverUrl={`/strategy-wall-${((idx % 4) + 1)}.svg`}
                  />
                  <div className="space-y-1 p-3">
                    <div className="truncate text-sm font-semibold">{card.name}</div>
                    <div className="text-xs" style={{ color: '#848E9C' }}>
                      {isZh ? '历史回测表现' : 'Historical Backtest'}
                    </div>
                    <div className="flex items-center justify-between text-xs">
                      <span style={{ color: '#0ECB81' }}>PnL +{card.pnl.toFixed(1)}%</span>
                      <span style={{ color: '#F6465D' }}>DD {card.dd.toFixed(1)}%</span>
                    </div>
                    <div className="pt-1 text-[11px]" style={{ color: card.runId ? '#F0B90B' : '#848E9C' }}>
                      {card.runId
                        ? isZh ? '点击查看该策略回测' : 'Click to view backtest'
                        : isZh ? '暂无匹配回测数据' : 'No matched run yet'}
                    </div>
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <div className="overflow-hidden">
              <div className="strategy-wall-track flex min-w-max gap-3">
                {[...wallCards, ...wallCards].map((card, idx) => (
                  <article
                    key={`${card.key}-${idx}`}
                    onClick={() => card.runId && setSelectedRunId(card.runId)}
                    className="w-[190px] sm:w-[220px] flex-shrink-0 overflow-hidden rounded-xl transition-all"
                    style={{
                      background: '#0B0E11',
                      border: '1px solid #2B3139',
                      cursor: card.runId ? 'pointer' : 'default',
                    }}
                  >
                    <MiniKlinePanel
                      runId={card.runId}
                      symbol={card.symbol}
                      coverUrl={`/strategy-wall-${((idx % 4) + 1)}.svg`}
                    />
                    <div className="space-y-1 p-3">
                      <div className="truncate text-sm font-semibold">{card.name}</div>
                      <div className="text-xs" style={{ color: '#848E9C' }}>
                        {isZh ? '历史回测表现' : 'Historical Backtest'}
                      </div>
                      <div className="flex items-center justify-between text-xs">
                        <span style={{ color: '#0ECB81' }}>PnL +{card.pnl.toFixed(1)}%</span>
                        <span style={{ color: '#F6465D' }}>DD {card.dd.toFixed(1)}%</span>
                      </div>
                      <div className="pt-1 text-[11px]" style={{ color: card.runId ? '#F0B90B' : '#848E9C' }}>
                        {card.runId
                          ? isZh ? '点击查看该策略回测' : 'Click to view backtest'
                          : isZh ? '暂无匹配回测数据' : 'No matched run yet'}
                      </div>
                    </div>
                  </article>
                ))}
              </div>
            </div>
          )}
        </section>

        <section className="mt-5 grid gap-4 md:grid-cols-3">
          <MetricCard icon={<Medal size={16} />} label={isZh ? '展示回测数' : 'Showcase Runs'} value={String(showcaseRuns.length)} />
          <MetricCard
            icon={<TrendingUp size={16} />}
            label={isZh ? '平均净值' : 'Avg Equity'}
            value={averageValue(showcaseRuns, (r) => r.summary?.equity_last ?? 0).toFixed(2)}
            suffix="USDT"
          />
          <MetricCard
            icon={<Activity size={16} />}
            label={isZh ? '平均最大回撤' : 'Avg Max DD'}
            value={`${averageValue(showcaseRuns, (r) => r.summary?.max_drawdown_pct ?? 0).toFixed(2)}%`}
          />
        </section>

        <section className="mt-5 grid gap-5 xl:grid-cols-[360px,1fr]">
          <aside className="rounded-xl p-4" style={{ background: '#11151B', border: '1px solid #2B3139' }}>
            <div className="mb-3 flex items-center gap-2 rounded-lg px-3 py-2" style={{ background: '#0B0E11', border: '1px solid #2B3139' }}>
              <Search size={14} style={{ color: '#848E9C' }} />
              <input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={isZh ? '搜索 run_id / 策略名' : 'Search run_id / strategy'}
                className="w-full bg-transparent text-sm outline-none"
                style={{ color: '#EAECEF' }}
              />
            </div>

            <div className="mb-3 flex flex-wrap gap-2">
              {(['all', 'done', 'running', 'paused', 'error'] as StateFilter[]).map((s) => (
                <button
                  key={s}
                  onClick={() => setStateFilter(s)}
                  className="rounded-md px-2 py-1 text-xs"
                  style={{
                    background: stateFilter === s ? 'rgba(240,185,11,0.18)' : '#1E2329',
                    border: stateFilter === s ? '1px solid rgba(240,185,11,0.7)' : '1px solid #2B3139',
                    color: stateFilter === s ? '#F0B90B' : '#AEB4BC',
                  }}
                >
                  {s === 'all' ? (isZh ? '全部' : 'All') : s}
                </button>
              ))}
            </div>

            <div className="max-h-[50vh] xl:max-h-[680px] space-y-2 overflow-y-auto pr-1">
              {runsLoading && <div className="py-8 text-center text-sm" style={{ color: '#848E9C' }}>{isZh ? '加载中...' : 'Loading...'}</div>}
              {!runsLoading && showcaseRuns.length === 0 && (
                <div className="py-8 text-center text-sm" style={{ color: '#848E9C' }}>
                  {isZh ? '暂无橱窗数据' : 'No showcase data'}
                </div>
              )}
              {showcaseRuns.map((run) => (
                <RunCard key={run.run_id} run={run} active={selectedRun?.run_id === run.run_id} onClick={() => setSelectedRunId(run.run_id)} />
              ))}
            </div>
          </aside>

          <main className="rounded-xl p-4 md:p-5" style={{ background: '#11151B', border: '1px solid #2B3139' }}>
            {!selectedRun ? (
              <div className="py-20 text-center" style={{ color: '#848E9C' }}>
                {isZh ? '请选择一个回测查看详情' : 'Select a run to view details'}
              </div>
            ) : (
              <div className="space-y-5">
                <header>
                  <div className="text-xs" style={{ color: '#848E9C' }}>RUN ID</div>
                  <h2 className="mt-1 font-mono text-sm md:text-base">{selectedRun.run_id}</h2>
                </header>

                <section className="grid gap-3 md:grid-cols-4">
                  <MiniStat label={isZh ? '最新净值' : 'Equity'} value={`${(selectedRun.summary?.equity_last ?? 0).toFixed(2)} USDT`} />
                  <MiniStat label={isZh ? '最大回撤' : 'Max DD'} value={`${(selectedRun.summary?.max_drawdown_pct ?? 0).toFixed(2)}%`} />
                  <MiniStat label={isZh ? '总收益率' : 'Return'} value={`${(metrics?.total_return_pct ?? 0).toFixed(2)}%`} />
                  <MiniStat label={isZh ? '胜率' : 'Win Rate'} value={`${(metrics?.win_rate ?? 0).toFixed(1)}%`} />
                </section>

                <section className="rounded-xl p-3" style={{ background: '#0B0E11', border: '1px solid #2B3139' }}>
                  <div className="mb-2 flex items-center gap-2 text-sm">
                    <LineChart size={15} style={{ color: '#F0B90B' }} />
                    {isZh ? '资金曲线' : 'Equity Curve'}
                  </div>
                  {equity && equity.length > 0 ? (
                    <BacktestChart equity={equity} trades={trades ?? []} />
                  ) : (
                    <div className="py-12 text-center text-sm" style={{ color: '#848E9C' }}>
                      {isZh ? '暂无资金曲线数据' : 'No equity data'}
                    </div>
                  )}
                  {selectedRun.run_id && (trades ?? []).length > 0 && (
                    <div className="mt-5">
                      <h4 className="mb-3 text-sm font-medium" style={{ color: '#EAECEF' }}>
                        {isZh ? 'K线图与交易标记' : 'Kline & Trade Markers'}
                      </h4>
                      <ShowcaseCandlestickChart runId={selectedRun.run_id} trades={trades ?? []} isZh={isZh} />
                    </div>
                  )}
                </section>

                <section className="grid gap-4 lg:grid-cols-2">
                  <div className="rounded-xl p-3" style={{ background: '#0B0E11', border: '1px solid #2B3139' }}>
                    <h3 className="mb-2 text-sm font-semibold">{isZh ? '最近成交' : 'Recent Trades'}</h3>
                    <div className="max-h-[220px] space-y-2 overflow-y-auto pr-1 text-xs">
                      {(trades ?? []).length === 0 && <div style={{ color: '#848E9C' }}>{isZh ? '暂无数据' : 'No data'}</div>}
                      {(trades ?? []).map((item, idx) => (
                        <div key={`${item.ts}-${item.symbol}-${idx}`} className="flex items-center justify-between rounded-lg px-2 py-2" style={{ background: '#11151B', border: '1px solid #2B3139' }}>
                          <div>
                            <div>{item.symbol} | {item.action}</div>
                            <div style={{ color: '#848E9C' }}>{new Date(item.ts).toLocaleString()}</div>
                          </div>
                          <div style={{ color: item.realized_pnl >= 0 ? '#0ECB81' : '#F6465D' }}>
                            {item.realized_pnl >= 0 ? '+' : ''}
                            {item.realized_pnl.toFixed(2)}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>

                  <div className="rounded-xl p-3" style={{ background: '#0B0E11', border: '1px solid #2B3139' }}>
                    <h3 className="mb-2 text-sm font-semibold">{isZh ? '最近决策' : 'Recent Decisions'}</h3>
                    <div className="max-h-[220px] space-y-2 overflow-y-auto pr-1 text-xs">
                      {(decisions ?? []).length === 0 && <div style={{ color: '#848E9C' }}>{isZh ? '暂无数据' : 'No data'}</div>}
                      {(decisions ?? []).map((item) => (
                        <div key={`${item.cycle_number}-${item.timestamp}`} className="rounded-lg px-2 py-2" style={{ background: '#11151B', border: '1px solid #2B3139' }}>
                          <div className="flex items-center justify-between">
                            <span style={{ color: '#F0B90B' }}>#{item.cycle_number}</span>
                            <span style={{ color: '#848E9C' }}>{new Date(item.timestamp).toLocaleString()}</span>
                          </div>
                          <div className="mt-1 line-clamp-2" style={{ color: '#AEB4BC' }}>
                            {renderDecisionSummary(item, isZh)}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                </section>
              </div>
            )}
          </main>
        </section>
      </div>

      <style>{`
        .wall-grid {
          display: grid;
          grid-template-columns: 1fr;
        }
        @media (min-width: 768px) {
          .wall-grid {
            grid-template-columns: repeat(var(--wall-cols), minmax(0, 1fr));
          }
        }
        .strategy-wall-track { animation: strategy-wall-right 36s linear infinite; will-change: transform; }
        .strategy-wall-track:hover { animation-play-state: paused; }
        @keyframes strategy-wall-right {
          0% { transform: translateX(-50%); }
          100% { transform: translateX(0%); }
        }
      `}</style>
    </div>
  )
}

function ShowcaseCandlestickChart({
  runId,
  trades,
  isZh,
}: {
  runId: string
  trades: BacktestTradeEvent[]
  isZh: boolean
}) {
  const chartContainerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<IChartApi | null>(null)
  const candleSeriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)

  const symbols = useMemo(() => {
    const symbolSet = new Set(trades.map((t) => t.symbol).filter(Boolean))
    return Array.from(symbolSet).sort()
  }, [trades])

  const [selectedSymbol, setSelectedSymbol] = useState<string>(symbols[0] || '')
  const [selectedTimeframe, setSelectedTimeframe] = useState<string>('15m')
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const chartTimeframes = ['1m', '3m', '5m', '15m', '30m', '1h', '4h', '1d']

  useEffect(() => {
    if (symbols.length > 0 && !symbols.includes(selectedSymbol)) {
      setSelectedSymbol(symbols[0])
    }
  }, [symbols, selectedSymbol])

  const symbolTrades = useMemo(() => trades.filter((t) => t.symbol === selectedSymbol), [trades, selectedSymbol])

  useEffect(() => {
    if (!chartContainerRef.current || !selectedSymbol || !runId) return

    const container = chartContainerRef.current
    const chart = createChart(container, {
      layout: {
        background: { type: ColorType.Solid, color: '#0B0E11' },
        textColor: '#848E9C',
      },
      grid: {
        vertLines: { color: 'rgba(43, 49, 57, 0.5)' },
        horzLines: { color: 'rgba(43, 49, 57, 0.5)' },
      },
      crosshair: { mode: CrosshairMode.Normal },
      rightPriceScale: { borderColor: '#2B3139' },
      timeScale: {
        borderColor: '#2B3139',
        timeVisible: true,
        secondsVisible: false,
      },
      width: container.clientWidth,
      height: 400,
    })

    chartRef.current = chart
    const candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: '#0ECB81',
      downColor: '#F6465D',
      borderUpColor: '#0ECB81',
      borderDownColor: '#F6465D',
      wickUpColor: '#0ECB81',
      wickDownColor: '#F6465D',
    })
    candleSeriesRef.current = candleSeries
    setIsLoading(true)
    setError(null)

    api
      .getBacktestKlines(runId, selectedSymbol, selectedTimeframe)
      .then((data: BacktestKlinesResponse) => {
        const klineData: CandlestickData<UTCTimestamp>[] = data.klines.map((k) => ({
          time: k.time as UTCTimestamp,
          open: k.open,
          high: k.high,
          low: k.low,
          close: k.close,
        }))
        candleSeries.setData(klineData)

        const markers: SeriesMarker<UTCTimestamp>[] = symbolTrades
          .map((trade) => {
            const tradeTime = Math.floor(trade.ts / 1000)
            const closestKline = data.klines.reduce((prev, curr) =>
              Math.abs(curr.time - tradeTime) < Math.abs(prev.time - tradeTime) ? curr : prev
            )
            const isOpen = trade.action.includes('open')
            const isLong = trade.side === 'long' || trade.action.includes('long')
            const pnl = trade.realized_pnl

            let text = ''
            let color = '#0ECB81'
            if (isOpen) {
              if (isLong) {
                text = `Long @${trade.price.toFixed(2)}`
                color = '#0ECB81'
              } else {
                text = `Short @${trade.price.toFixed(2)}`
                color = '#F6465D'
              }
            } else {
              const pnlStr = pnl >= 0 ? `+$${pnl.toFixed(2)}` : `-$${Math.abs(pnl).toFixed(2)}`
              text = `PnL ${pnlStr}`
              color = pnl >= 0 ? '#0ECB81' : '#F6465D'
            }

            return {
              time: closestKline.time as UTCTimestamp,
              position: isOpen
                ? (isLong ? 'belowBar' as const : 'aboveBar' as const)
                : (isLong ? 'aboveBar' as const : 'belowBar' as const),
              color,
              shape: 'circle' as const,
              size: 2,
              text,
            }
          })
          .sort((a, b) => (a.time as number) - (b.time as number))

        createSeriesMarkers(candleSeries, markers)
        chart.timeScale().fitContent()
        setIsLoading(false)
      })
      .catch((err) => {
        setError(err?.message || (isZh ? '加载K线数据失败' : 'Failed to load kline data'))
        setIsLoading(false)
      })

    const handleResize = () => {
      if (chartContainerRef.current) {
        chart.applyOptions({ width: chartContainerRef.current.clientWidth })
      }
    }
    window.addEventListener('resize', handleResize)

    return () => {
      window.removeEventListener('resize', handleResize)
      chart.remove()
      chartRef.current = null
      candleSeriesRef.current = null
    }
  }, [isZh, runId, selectedSymbol, selectedTimeframe, symbolTrades])

  if (symbols.length === 0) {
    return (
      <div className="py-10 text-center text-sm" style={{ color: '#848E9C' }}>
        {isZh ? '暂无交易数据用于展示标记' : 'No trade data available for chart markers'}
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-4">
        <div className="flex items-center gap-2">
          <CandlestickIcon size={16} style={{ color: '#F0B90B' }} />
          <span className="text-sm" style={{ color: '#848E9C' }}>
            {isZh ? '币种' : 'Symbol'}
          </span>
          <select
            value={selectedSymbol}
            onChange={(e) => setSelectedSymbol(e.target.value)}
            className="rounded px-3 py-1.5 text-sm"
            style={{ background: '#1E2329', border: '1px solid #2B3139', color: '#EAECEF' }}
          >
            {symbols.map((sym) => (
              <option key={sym} value={sym}>
                {sym}
              </option>
            ))}
          </select>
        </div>

        <div className="flex items-center gap-2">
          <Clock size={14} style={{ color: '#848E9C' }} />
          <span className="text-sm" style={{ color: '#848E9C' }}>
            {isZh ? '周期' : 'Interval'}
          </span>
          <div className="flex overflow-hidden rounded" style={{ border: '1px solid #2B3139' }}>
            {chartTimeframes.map((tf) => (
              <button
                key={tf}
                onClick={() => setSelectedTimeframe(tf)}
                className="px-2.5 py-1 text-xs font-medium transition-colors"
                style={{
                  background: selectedTimeframe === tf ? '#F0B90B' : '#1E2329',
                  color: selectedTimeframe === tf ? '#0B0E11' : '#848E9C',
                }}
              >
                {tf}
              </button>
            ))}
          </div>
        </div>

        <span className="text-xs" style={{ color: '#5E6673' }}>
          ({symbolTrades.length} {isZh ? '笔交易' : 'trades'})
        </span>
      </div>

      <div
        ref={chartContainerRef}
        className="w-full overflow-hidden rounded-lg"
        style={{ background: '#0B0E11', minHeight: 400 }}
      >
        {isLoading && (
          <div className="flex h-[400px] items-center justify-center" style={{ color: '#848E9C' }}>
            <RefreshCw className="mr-2 animate-spin" size={16} />
            {isZh ? '加载K线数据中...' : 'Loading kline data...'}
          </div>
        )}
        {error && (
          <div className="flex h-[400px] items-center justify-center" style={{ color: '#F6465D' }}>
            <AlertTriangle className="mr-2" size={16} />
            {error}
          </div>
        )}
      </div>
    </div>
  )
}

// Same chart style/behavior as original BacktestPage chart.
function BacktestChart({
  equity,
  trades,
}: {
  equity: BacktestEquityPoint[]
  trades: BacktestTradeEvent[]
}) {
  const chartData = useMemo(() => {
    return equity.map((point) => ({
      time: new Date(point.ts).toLocaleString(),
      ts: point.ts,
      equity: point.equity,
      pnl_pct: point.pnl_pct,
    }))
  }, [equity])

  const tradeMarkers = useMemo(() => {
    if (!trades.length || !equity.length) return []
    return trades
      .filter((t) => t.action.includes('open') || t.action.includes('close'))
      .map((trade) => {
        const closest = equity.reduce((prev, curr) =>
          Math.abs(curr.ts - trade.ts) < Math.abs(prev.ts - trade.ts) ? curr : prev
        )
        return {
          ts: closest.ts,
          equity: closest.equity,
          isOpen: trade.action.includes('open'),
        }
      })
      .slice(-30)
  }, [trades, equity])

  return (
    <div className="h-[300px] w-full">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={chartData} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id="equityGradient" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#F0B90B" stopOpacity={0.4} />
              <stop offset="95%" stopColor="#F0B90B" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="rgba(43, 49, 57, 0.5)" strokeDasharray="3 3" />
          <XAxis
            dataKey="time"
            tick={{ fill: '#848E9C', fontSize: 10 }}
            axisLine={{ stroke: '#2B3139' }}
            tickLine={{ stroke: '#2B3139' }}
            hide
          />
          <YAxis
            tick={{ fill: '#848E9C', fontSize: 10 }}
            axisLine={{ stroke: '#2B3139' }}
            tickLine={{ stroke: '#2B3139' }}
            width={60}
            domain={['auto', 'auto']}
          />
          <Tooltip
            contentStyle={{
              background: '#1E2329',
              border: '1px solid #2B3139',
              borderRadius: 8,
              color: '#EAECEF',
            }}
            labelStyle={{ color: '#848E9C' }}
            formatter={(value: number) => [`$${value.toFixed(2)}`, 'Equity']}
          />
          <Area
            type="monotone"
            dataKey="equity"
            stroke="#F0B90B"
            strokeWidth={2}
            fill="url(#equityGradient)"
            dot={false}
            activeDot={{ r: 4, fill: '#F0B90B' }}
          />
          {tradeMarkers.map((marker, idx) => (
            <ReferenceDot
              key={`${marker.ts}-${idx}`}
              x={chartData.findIndex((d) => d.ts === marker.ts)}
              y={marker.equity}
              r={4}
              fill={marker.isOpen ? '#0ECB81' : '#F6465D'}
              stroke={marker.isOpen ? '#0ECB81' : '#F6465D'}
            />
          ))}
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

function MiniKlinePanel({ runId, symbol, coverUrl }: { runId: string; symbol: string; coverUrl: string }) {
  const { data: fallbackTrades } = useSWR<BacktestTradeEvent[]>(
    runId && !symbol ? ['wall-kline-fallback-symbol', runId] : null,
    () => api.getBacktestTrades(runId, 1),
    { revalidateOnFocus: false }
  )
  const resolvedSymbol = symbol || fallbackTrades?.[0]?.symbol || ''
  const canLoad = !!runId && !!resolvedSymbol
  const { data } = useSWR<BacktestKlinesResponse>(
    canLoad ? ['wall-kline', runId, resolvedSymbol] : null,
    () => api.getBacktestKlines(runId, resolvedSymbol, '15m'),
    { revalidateOnFocus: false }
  )

  const candles = useMemo(() => {
    const items = data?.klines || []
    if (!items.length) return []
    return items.slice(-24)
  }, [data])

  if (!candles.length) {
    return (
      <img src={coverUrl} alt="strategy cover" className="h-28 w-full object-cover" />
    )
  }

  const low = Math.min(...candles.map((c) => c.low))
  const high = Math.max(...candles.map((c) => c.high))
  const span = Math.max(1e-9, high - low)
  const w = 220
  const h = 112
  const padY = 8
  const step = w / Math.max(1, candles.length)
  const bodyW = Math.max(2, step * 0.55)
  const scaleY = (v: number) => padY + ((high - v) / span) * (h - padY * 2)

  return (
    <svg viewBox={`0 0 ${w} ${h}`} className="h-28 w-full" preserveAspectRatio="none">
      <rect x="0" y="0" width={w} height={h} fill="#09121D" />
      {candles.map((candle, idx) => (
        <MiniCandle
          key={`${candle.time}-${idx}`}
          candle={candle}
          x={idx * step + step / 2}
          bodyW={bodyW}
          scaleY={scaleY}
        />
      ))}
    </svg>
  )
}

function MiniCandle({
  candle,
  x,
  bodyW,
  scaleY,
}: {
  candle: BacktestKline
  x: number
  bodyW: number
  scaleY: (v: number) => number
}) {
  const up = candle.close >= candle.open
  const color = up ? '#0ECB81' : '#F6465D'
  const yOpen = scaleY(candle.open)
  const yClose = scaleY(candle.close)
  const bodyY = Math.min(yOpen, yClose)
  const bodyH = Math.max(1.2, Math.abs(yClose - yOpen))
  const yHigh = scaleY(candle.high)
  const yLow = scaleY(candle.low)

  return (
    <g>
      <line x1={x} x2={x} y1={yHigh} y2={yLow} stroke={color} strokeWidth="1" />
      <rect x={x - bodyW / 2} y={bodyY} width={bodyW} height={bodyH} fill={color} rx="0.8" />
    </g>
  )
}

function MetricCard({ icon, label, value, suffix }: { icon: ReactNode; label: string; value: string; suffix?: string }) {
  return (
    <div className="rounded-xl p-4" style={{ background: '#11151B', border: '1px solid #2B3139' }}>
      <div className="mb-2 flex items-center gap-2 text-xs" style={{ color: '#848E9C' }}>{icon}{label}</div>
      <div className="text-2xl font-semibold">
        {value}
        {suffix ? <span className="ml-1 text-sm text-[#848E9C]">{suffix}</span> : null}
      </div>
    </div>
  )
}

function MiniStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg px-3 py-2" style={{ background: '#0B0E11', border: '1px solid #2B3139' }}>
      <div className="text-xs" style={{ color: '#848E9C' }}>{label}</div>
      <div className="mt-1 text-sm font-semibold">{value}</div>
    </div>
  )
}

function RunCard({ run, active, onClick }: { run: BacktestRunMetadata; active: boolean; onClick: () => void }) {
  const color = stateColor[run.state] ?? '#848E9C'
  return (
    <button
      onClick={onClick}
      className="w-full rounded-lg p-3 text-left"
      style={{
        background: active ? 'rgba(240,185,11,0.12)' : '#1E2329',
        border: active ? '1px solid #F0B90B' : '1px solid #2B3139',
      }}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="truncate font-mono text-xs">{run.run_id}</div>
        <span className="rounded-full px-2 py-0.5 text-[11px]" style={{ background: `${color}22`, color, border: `1px solid ${color}55` }}>
          {run.state}
        </span>
      </div>
      <div className="mt-1 truncate text-xs" style={{ color: '#AEB4BC' }}>
        {getPremiumStrategyName(run.strategy_name || 'Manual')}
      </div>
      <div className="mt-2 flex items-center justify-between text-[11px]" style={{ color: '#848E9C' }}>
        <span>DD {(run.summary?.max_drawdown_pct ?? 0).toFixed(2)}%</span>
        <span>${(run.summary?.equity_last ?? 0).toFixed(2)}</span>
      </div>
    </button>
  )
}

function averageValue(items: BacktestRunMetadata[], getter: (r: BacktestRunMetadata) => number): number {
  if (!items.length) return 0
  const total = items.reduce((sum, item) => sum + getter(item), 0)
  return total / items.length
}

function renderDecisionSummary(item: DecisionRecord, isZh: boolean): string {
  const actions = extractDecisionActions(item)
  if (actions.length === 0) {
    return item.error_message || (isZh ? '决策执行完成' : 'Decision executed')
  }
  const chunks = actions.slice(0, 2).map(formatDecisionAction)
  if (actions.length > 2) {
    chunks.push(isZh ? `等${actions.length}条` : `+${actions.length - 2} more`)
  }
  return chunks.join(' | ')
}

function extractDecisionActions(item: DecisionRecord): DecisionAction[] {
  if (Array.isArray(item.decisions) && item.decisions.length > 0) return item.decisions
  const payload = parseJSONSafe(item.decision_json)
  if (!payload || typeof payload !== 'object') return []
  const raw = (payload as { decisions?: unknown }).decisions
  if (!Array.isArray(raw)) return []
  return raw
    .filter((x): x is Record<string, unknown> => !!x && typeof x === 'object')
    .map((x) => ({
      action: stringValue(x.action),
      symbol: stringValue(x.symbol),
      quantity: numberValue(x.quantity),
      leverage: numberValue(x.leverage),
      confidence: numberValue(x.confidence),
      price: numberValue(x.price),
      order_id: numberValue(x.order_id),
      timestamp: stringValue(x.timestamp),
      success: true,
    }))
    .filter((x) => x.action || x.symbol)
}

function formatDecisionAction(action: DecisionAction): string {
  const act = action.action || '-'
  const sym = action.symbol || '-'
  const lev = action.leverage ? `x${action.leverage}` : ''
  const qty = action.quantity ? `q:${trimNum(action.quantity)}` : ''
  const conf = action.confidence ? `c:${trimNum(action.confidence)}` : ''
  return [act, sym, lev, qty, conf].filter(Boolean).join(' ')
}

function parseJSONSafe(value: string): unknown {
  if (!value) return null
  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function numberValue(value: unknown): number {
  const n = Number(value)
  return Number.isFinite(n) ? n : 0
}

function trimNum(v: number): string {
  if (!Number.isFinite(v)) return '0'
  return Number.isInteger(v) ? String(v) : v.toFixed(2)
}
