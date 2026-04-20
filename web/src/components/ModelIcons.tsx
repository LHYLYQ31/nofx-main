import { useState } from 'react'
import { withBasePath } from '../utils/assetPath'

interface IconProps {
  width?: number
  height?: number
  className?: string
}

const MODEL_COLORS: Record<string, string> = {
  deepseek: '#4A90E2',
  qwen: '#9B59B6',
  claude: '#D97757',
  kimi: '#6366F1',
  gemini: '#4285F4',
  grok: '#000000',
  openai: '#10A37F',
  minimax: '#E45735',
  'blockrun-base': '#2563EB',
  'blockrun-sol': '#9945FF',
}

const MODEL_ICON_PATHS: Record<string, string> = {
  deepseek: '/icons/deepseek.svg',
  qwen: '/icons/qwen.svg',
  claude: '/icons/claude.svg',
  kimi: '/icons/kimi.svg',
  gemini: '/icons/gemini.svg',
  grok: '/icons/grok.svg',
  openai: '/icons/openai.svg',
  minimax: '/icons/minimax.svg',
  'blockrun-base': '/icons/blockrun.svg',
  'blockrun-sol': '/icons/blockrun.svg',
}

function normalizeModelType(modelType: string): string {
  const rawType = modelType.includes('_') ? modelType.split('_').pop() || '' : modelType
  return rawType.toLowerCase()
}

function ModelFallback({ type, width = 24, height = 24, className }: { type: string } & IconProps) {
  return (
    <div
      className={className}
      style={{
        width,
        height,
        borderRadius: '50%',
        background: MODEL_COLORS[type] || '#60a5fa',
        color: '#fff',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: Math.max(10, (width || 24) * 0.45),
        fontWeight: 700,
      }}
    >
      {type[0]?.toUpperCase() || '?'}
    </div>
  )
}

function ModelImage({ type, iconPath, width = 24, height = 24, className }: { type: string; iconPath: string } & IconProps) {
  const [hasError, setHasError] = useState(false)

  if (hasError) {
    return <ModelFallback type={type} width={width} height={height} className={className} />
  }

  return (
    <img
      src={withBasePath(iconPath)}
      alt={`${type} icon`}
      width={width}
      height={height}
      className={className}
      onError={() => setHasError(true)}
    />
  )
}

export const getModelIcon = (modelType: string, props: IconProps = {}) => {
  const type = normalizeModelType(modelType)
  const iconPath = MODEL_ICON_PATHS[type]

  if (!iconPath) {
    return null
  }

  return (
    <ModelImage
      type={type}
      iconPath={iconPath}
      width={props.width || 24}
      height={props.height || 24}
      className={props.className}
    />
  )
}

export const getModelColor = (modelType: string): string => {
  const type = normalizeModelType(modelType)
  return MODEL_COLORS[type] || '#60a5fa'
}
