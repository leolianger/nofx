import { useState, useRef, useEffect } from 'react'
import { useLanguage } from '../contexts/LanguageContext'

interface Message {
  role: 'user' | 'bot'
  text: string
  time: string
}

export function AgentChatPage() {
  const { language } = useLanguage()
  const [messages, setMessages] = useState<Message[]>([
    { role: 'bot', text: language === 'zh' 
      ? '👋 你好！我是 NOFXi，你的 AI 交易 Agent。\n\n跟我说话就行，我能帮你分析市场、管理交易、配置策略。\n\n试试: "分析一下BTC" 或 /help' 
      : '👋 Hi! I\'m NOFXi, your AI trading agent.\n\nJust talk to me. I can analyze markets, manage trades, and configure strategies.\n\nTry: "Analyze BTC" or /help',
      time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) }
  ])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [composing, setComposing] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const send = async (text?: string) => {
    const msg = text || input.trim()
    if (!msg || loading) return
    setInput('')
    const time = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    setMessages(prev => [...prev, { role: 'user', text: msg, time }])
    setLoading(true)

    try {
      const res = await fetch('/api/agent/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: msg, user_id: 1, lang: language })
      })
      const data = await res.json()
      setMessages(prev => [...prev, { 
        role: 'bot', 
        text: data.response || data.error || 'No response',
        time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
      }])
    } catch (e: any) {
      setMessages(prev => [...prev, { role: 'bot', text: '⚠️ Error: ' + e.message, time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) }])
    }
    setLoading(false)
    inputRef.current?.focus()
  }

  const quickActions = language === 'zh' 
    ? [
        { label: '📊 分析 BTC', cmd: '/analyze BTC' },
        { label: '📊 分析 ETH', cmd: '/analyze ETH' },
        { label: '💼 持仓', cmd: '/positions' },
        { label: '💰 余额', cmd: '/balance' },
        { label: '📋 Traders', cmd: '/traders' },
        { label: '❓ 帮助', cmd: '/help' },
      ]
    : [
        { label: '📊 Analyze BTC', cmd: '/analyze BTC' },
        { label: '📊 Analyze ETH', cmd: '/analyze ETH' },
        { label: '💼 Positions', cmd: '/positions' },
        { label: '💰 Balance', cmd: '/balance' },
        { label: '📋 Traders', cmd: '/traders' },
        { label: '❓ Help', cmd: '/help' },
      ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#09090b' }}>
      {/* Quick actions */}
      <div style={{ display: 'flex', gap: 6, padding: '10px 16px', borderBottom: '1px solid #1f1f2c', overflowX: 'auto', flexShrink: 0 }}>
        {quickActions.map((a, i) => (
          <button key={i} onClick={() => send(a.cmd)}
            style={{ padding: '5px 12px', background: '#13131b', border: '1px solid #1f1f2c', borderRadius: 8, color: '#a0a0b0', fontSize: 12, cursor: 'pointer', whiteSpace: 'nowrap', fontFamily: 'inherit' }}
          >{a.label}</button>
        ))}
      </div>

      {/* Messages */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '16px 20px' }}>
        {messages.map((m, i) => (
          <div key={i} style={{ display: 'flex', gap: 10, marginBottom: 14, flexDirection: m.role === 'user' ? 'row-reverse' : 'row' }}>
            <div style={{ width: 30, height: 30, borderRadius: 8, display: 'grid', placeItems: 'center', fontSize: 15, flexShrink: 0, background: m.role === 'user' ? 'rgba(139,92,246,.1)' : 'rgba(0,229,160,.05)', border: '1px solid ' + (m.role === 'user' ? 'rgba(139,92,246,.15)' : '#1f1f2c') }}>
              {m.role === 'user' ? '👤' : '⚡'}
            </div>
            <div style={{ maxWidth: '70%' }}>
              <div style={{ padding: '10px 14px', borderRadius: 14, fontSize: 13, lineHeight: 1.65, whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                background: m.role === 'user' ? 'linear-gradient(135deg, #8b5cf6, #6d28d9)' : '#13131b',
                color: m.role === 'user' ? '#fff' : '#eaeaf0',
                border: m.role === 'bot' ? '1px solid #1f1f2c' : 'none',
                borderTopLeftRadius: m.role === 'bot' ? 3 : 14,
                borderTopRightRadius: m.role === 'user' ? 3 : 14,
              }}>{m.text}</div>
              <div style={{ fontSize: 10, color: '#5c5c72', marginTop: 2, textAlign: m.role === 'user' ? 'right' : 'left' }}>{m.time}</div>
            </div>
          </div>
        ))}
        {loading && (
          <div style={{ display: 'flex', gap: 10, marginBottom: 14 }}>
            <div style={{ width: 30, height: 30, borderRadius: 8, display: 'grid', placeItems: 'center', fontSize: 15, background: 'rgba(0,229,160,.05)', border: '1px solid #1f1f2c' }}>⚡</div>
            <div style={{ padding: '10px 14px', background: '#13131b', border: '1px solid #1f1f2c', borderRadius: 14, borderTopLeftRadius: 3, color: '#5c5c72', fontSize: 13 }}>
              {language === 'zh' ? '思考中...' : 'Thinking...'}
            </div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div style={{ padding: '12px 16px 16px', borderTop: '1px solid #1f1f2c', background: '#0c0c12' }}>
        <div style={{ display: 'flex', gap: 8, maxWidth: 800, margin: '0 auto', background: '#13131b', border: '1px solid #1f1f2c', borderRadius: 14, padding: '3px 3px 3px 14px' }}>
          <input ref={inputRef} value={input} onChange={e => setInput(e.target.value)}
            onCompositionStart={() => setComposing(true)}
            onCompositionEnd={() => setComposing(false)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !composing) { e.preventDefault(); send() } }}
            placeholder={language === 'zh' ? '跟 NOFXi 聊点什么...' : 'Ask NOFXi anything...'}
            style={{ flex: 1, background: 'none', border: 'none', color: '#eaeaf0', fontSize: 13, outline: 'none', padding: '8px 0', fontFamily: 'inherit' }}
          />
          <button onClick={() => send()} disabled={loading}
            style={{ width: 36, height: 36, borderRadius: 10, border: 'none', background: 'linear-gradient(135deg, #00e5a0, #00c896)', color: '#000', fontSize: 16, cursor: loading ? 'not-allowed' : 'pointer', opacity: loading ? .35 : 1, display: 'grid', placeItems: 'center', flexShrink: 0 }}
          >➤</button>
        </div>
      </div>
    </div>
  )
}
