import { useState, useEffect } from 'react'
import { TrendingUp, TrendingDown, RefreshCw } from 'lucide-react'

interface TickerData {
  symbol: string
  lastPrice: string
  priceChangePercent: string
  highPrice: string
  lowPrice: string
  volume: string
}

const SYMBOLS = ['BTCUSDT', 'ETHUSDT', 'SOLUSDT']

export function MarketTicker() {
  const [tickers, setTickers] = useState<Record<string, TickerData>>({})
  const [loading, setLoading] = useState(true)

  const fetchTickers = async () => {
    try {
      const results = await Promise.all(
        SYMBOLS.map(async (symbol) => {
          const res = await fetch(`/api/agent/ticker?symbol=${symbol}`)
          const data = await res.json()
          return { symbol, ...data }
        })
      )
      const map: Record<string, TickerData> = {}
      results.forEach((r) => {
        if (r.lastPrice) map[r.symbol] = r
      })
      setTickers(map)
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchTickers()
    const interval = setInterval(fetchTickers, 15000)
    return () => clearInterval(interval)
  }, [])

  const formatPrice = (price: string) => {
    const n = parseFloat(price)
    if (n >= 1000) return n.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
    if (n >= 1) return n.toFixed(2)
    return n.toFixed(4)
  }

  const formatVolume = (vol: string) => {
    const n = parseFloat(vol)
    if (n >= 1e9) return (n / 1e9).toFixed(1) + 'B'
    if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M'
    if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K'
    return n.toFixed(0)
  }

  if (loading) {
    return (
      <div style={{ padding: '12px 14px', color: '#5c5c72', fontSize: 12 }}>
        <RefreshCw size={14} className="animate-spin inline mr-2" />
        Loading market data...
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      {SYMBOLS.map((sym) => {
        const t = tickers[sym]
        if (!t) return null
        const pct = parseFloat(t.priceChangePercent)
        const isUp = pct >= 0
        const color = isUp ? '#00e5a0' : '#F6465D'
        const label = sym.replace('USDT', '')

        return (
          <div
            key={sym}
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '10px 12px',
              background: '#0d0d15',
              borderRadius: 10,
              border: '1px solid #1a1a28',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <div
                style={{
                  width: 28,
                  height: 28,
                  borderRadius: 7,
                  background: isUp ? 'rgba(0,229,160,0.08)' : 'rgba(246,70,93,0.08)',
                  display: 'grid',
                  placeItems: 'center',
                }}
              >
                {isUp ? (
                  <TrendingUp size={14} color={color} />
                ) : (
                  <TrendingDown size={14} color={color} />
                )}
              </div>
              <div>
                <div style={{ fontSize: 13, fontWeight: 600, color: '#eaeaf0' }}>
                  {label}
                </div>
                <div style={{ fontSize: 10, color: '#5c5c72' }}>
                  Vol {formatVolume(t.volume)}
                </div>
              </div>
            </div>
            <div style={{ textAlign: 'right' }}>
              <div style={{ fontSize: 13, fontWeight: 600, color: '#eaeaf0' }}>
                ${formatPrice(t.lastPrice)}
              </div>
              <div style={{ fontSize: 11, fontWeight: 500, color }}>
                {isUp ? '+' : ''}{pct.toFixed(2)}%
              </div>
            </div>
          </div>
        )
      })}
    </div>
  )
}
