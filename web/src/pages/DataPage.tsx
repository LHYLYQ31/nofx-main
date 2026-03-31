import { ExternalLink, BarChart3, ShieldAlert } from 'lucide-react'
import { useLanguage } from '../contexts/LanguageContext'

export function DataPage() {
  const { language } = useLanguage()
  const isZh = language === 'zh'
  const isId = language === 'id'

  const title = isZh ? '数据中心' : isId ? 'Pusat Data' : 'Data Center'
  const subtitle = isZh
    ? '官网看板禁止被 iframe 嵌入，因此改为外链打开。'
    : isId
      ? 'Dashboard resmi memblokir embedding iframe, jadi dibuka lewat tautan eksternal.'
      : 'The official dashboard blocks iframe embedding, so it is opened via an external link.'
  const openDashboard = isZh ? '打开官网数据看板' : isId ? 'Buka Dashboard Resmi' : 'Open Official Dashboard'
  const openDocs = isZh ? '打开 API 文档' : isId ? 'Buka Dokumen API' : 'Open API Docs'
  const tip = isZh
    ? '说明：这里不再内嵌 nofxos.ai，避免浏览器“已拒绝连接”报错。'
    : isId
      ? 'Catatan: Halaman ini tidak lagi menyematkan nofxos.ai untuk menghindari error "refused to connect".'
      : 'Note: This page no longer embeds nofxos.ai to avoid "refused to connect" browser errors.'

  return (
    <div className="w-full min-h-[calc(100dvh-64px)] p-3 sm:p-4 md:p-8">
      <div
        className="max-w-4xl mx-auto rounded-2xl p-4 sm:p-6 md:p-8"
        style={{
          background: 'rgba(17, 21, 27, 0.92)',
          border: '1px solid #2B3139',
        }}
      >
        <div className="flex items-start gap-3 mb-4">
          <BarChart3 className="w-6 h-6 mt-0.5" style={{ color: '#F0B90B' }} />
          <div>
            <h1 className="text-xl md:text-2xl font-bold" style={{ color: '#EAECEF' }}>
              {title}
            </h1>
            <p className="text-sm mt-1" style={{ color: '#848E9C' }}>
              {subtitle}
            </p>
          </div>
        </div>

        <div
          className="rounded-xl p-4 mb-5 flex items-start gap-2"
          style={{ background: 'rgba(240, 185, 11, 0.08)', border: '1px solid rgba(240, 185, 11, 0.25)' }}
        >
          <ShieldAlert className="w-4 h-4 mt-0.5" style={{ color: '#F0B90B' }} />
          <p className="text-sm" style={{ color: '#EAECEF' }}>
            {tip}
          </p>
        </div>

        <div className="flex flex-col sm:flex-row gap-3">
          <a
            href="https://nofxos.ai/dashboard"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-lg font-medium w-full sm:w-auto"
            style={{ background: '#F0B90B', color: '#0B0E11' }}
          >
            {openDashboard}
            <ExternalLink className="w-4 h-4" />
          </a>
          <a
            href="https://nofxos.ai/api-docs"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-lg font-medium w-full sm:w-auto"
            style={{ background: '#1E2329', border: '1px solid #2B3139', color: '#EAECEF' }}
          >
            {openDocs}
            <ExternalLink className="w-4 h-4" />
          </a>
        </div>
      </div>
    </div>
  )
}
