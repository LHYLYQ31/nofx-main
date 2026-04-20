import React, { useState } from 'react'
import { withBasePath } from '../utils/assetPath'

interface IconProps {
  width?: number
  height?: number
  className?: string
}

const ICON_PATHS: Record<string, string> = {
  binance: '/exchange-icons/binance.jpg',
  bybit: '/exchange-icons/bybit.png',
  okx: '/exchange-icons/okx.svg',
  bitget: '/exchange-icons/bitget.svg',
  gate: '/exchange-icons/gate.svg',
  kucoin: '/exchange-icons/kucoin.svg',
  hyperliquid: '/exchange-icons/hyperliquid.png',
  aster: '/exchange-icons/aster.svg',
  lighter: '/exchange-icons/lighter.png',
  indodax: '/exchange-icons/indodax.png',
}

const FallbackIcon: React.FC<IconProps & { label: string }> = ({
  width = 24,
  height = 24,
  className,
  label,
}) => (
  <div
    className={className}
    style={{
      width,
      height,
      borderRadius: 6,
      background: '#2B3139',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      fontSize: Math.max(10, (width || 24) * 0.4),
      fontWeight: 'bold',
      color: '#EAECEF',
      flexShrink: 0,
    }}
  >
    {label[0]?.toUpperCase() || '?'}
  </div>
)

const ExchangeImage: React.FC<IconProps & { src: string; alt: string }> = ({
  width = 24,
  height = 24,
  className,
  src,
  alt,
}) => {
  const [hasError, setHasError] = useState(false)

  if (hasError) {
    return <FallbackIcon width={width} height={height} className={className} label={alt} />
  }

  return (
    <div
      className={className}
      style={{
        width,
        height,
        borderRadius: 6,
        overflow: 'hidden',
        flexShrink: 0,
        background: '#2B3139',
      }}
    >
      <img
        src={src}
        alt={alt}
        onError={() => setHasError(true)}
        style={{
          width: '100%',
          height: '100%',
          objectFit: 'cover',
        }}
      />
    </div>
  )
}

export const getExchangeIcon = (exchangeType: string, props: IconProps = {}) => {
  const lowerType = exchangeType.toLowerCase()
  const type = lowerType.includes('binance')
    ? 'binance'
    : lowerType.includes('bybit')
      ? 'bybit'
      : lowerType.includes('okx')
        ? 'okx'
        : lowerType.includes('bitget')
          ? 'bitget'
          : lowerType.includes('gate')
            ? 'gate'
            : lowerType.includes('kucoin')
              ? 'kucoin'
              : lowerType.includes('hyperliquid')
                ? 'hyperliquid'
                : lowerType.includes('aster')
                  ? 'aster'
                  : lowerType.includes('lighter')
                    ? 'lighter'
                    : lowerType.includes('indodax')
                      ? 'indodax'
                      : lowerType

  const iconProps = {
    width: props.width || 24,
    height: props.height || 24,
    className: props.className,
  }

  const path = ICON_PATHS[type]
  if (path) {
    return <ExchangeImage {...iconProps} src={withBasePath(path)} alt={type} />
  }

  return <FallbackIcon {...iconProps} label={type} />
}
