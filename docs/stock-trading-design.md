# NOFXi 股票交易能力设计方案

> 创建: 2026-03-23 13:00
> 状态: 方案设计阶段
> 目标: 让 NOFXi 具备美股、港股、A股的实际交易能力

## 一、市场分析

| 市场 | 难度 | 合规要求 | 交易时段 | 结算 |
|------|------|---------|---------|------|
| 美股 | ⭐⭐ | 较宽松，API 友好 | 盘前4:00-9:30 / 正常9:30-16:00 / 盘后16:00-20:00 ET | T+1 |
| 港股 | ⭐⭐⭐ | 需持牌券商 | 9:30-12:00 / 13:00-16:00 HKT | T+2 |
| A股 | ⭐⭐⭐⭐⭐ | 极其严格，程序化交易有准入门槛 | 9:30-11:30 / 13:00-15:00 CST | T+1（卖出T+0到账） |

## 二、券商/API 选型对比

### 美股交易

| 方案 | 优点 | 缺点 | API 风格 | 费用 |
|------|------|------|---------|------|
| **Alpaca** ⭐推荐 | REST API 极其简洁，Paper Trading 免费，支持盘前盘后，Go SDK | 仅美股 | REST + WebSocket | 免佣金 |
| **Interactive Brokers (IBKR)** | 全球市场（美/港/A股通），最全面 | API 复杂(TWS/Gateway)，需要本地运行 | Java/C++/Python/REST | 低佣金 |
| **Tradier** | REST API 友好 | 仅美股，知名度低 | REST | $0 佣金(Sandbox) |

### 港股交易

| 方案 | 优点 | 缺点 | API 风格 | 费用 |
|------|------|------|---------|------|
| **LongPort (长桥)** ⭐推荐 | 专为程序化设计，Go/Rust SDK，美股+港股 | 需开户 | REST + WebSocket + gRPC | 低佣 |
| **Futu OpenD (富途)** | 中文社区大，港股+美股+A股 | 需运行 OpenD 网关 | TCP Protocol + REST | 低佣 |
| **Tiger (老虎)** | 美股+港股，Python SDK | API 不如长桥 | REST | 中等 |
| **IBKR** | 通过港股通可交易 | 设置复杂 | 同上 | 低佣 |

### A股交易

| 方案 | 优点 | 缺点 | 合规风险 |
|------|------|------|---------|
| **Futu OpenD** | 有 A 股通道（沪港通/深港通） | 需 OpenD 网关，间接交易 | 低 |
| **IBKR 沪港通** | 通过港股通交易 A 股 | 品种有限 | 低 |
| **掘金量化 / QMT** | 国内正规量化平台 | 需机构资质或高门槛 | 合规 |
| **通达信/同花顺接口** | 直接对接国内券商 | 灰色地带，券商可能封号 | ⚠️ 高风险 |

## 三、推荐架构

### Phase 1: 美股交易（Alpaca）— 2-3天
```
用户 → NOFXi Agent → trader/alpaca/ → Alpaca API
                                        (Paper → Live)
```
**为什么选 Alpaca：**
- REST API 极简，和 NOFX 现有 Trader 接口完美匹配
- Paper Trading 环境免费，0 风险测试
- 支持盘前盘后交易
- 免佣金
- Go HTTP 调用即可，无需 SDK

**核心实现：**
```go
// trader/alpaca/trader.go
type AlpacaTrader struct {
    apiKey    string
    apiSecret string
    baseURL   string  // paper-api.alpaca.markets or api.alpaca.markets
    client    *http.Client
}

// 实现 Trader interface
func (t *AlpacaTrader) OpenLong(symbol string, qty float64, leverage int) (map[string]interface{}, error)
func (t *AlpacaTrader) CloseLong(symbol string, qty float64) (map[string]interface{}, error)
func (t *AlpacaTrader) GetPositions() ([]map[string]interface{}, error)
func (t *AlpacaTrader) GetBalance() (map[string]interface{}, error)
func (t *AlpacaTrader) GetMarketPrice(symbol string) (float64, error)
// ... 美股不支持做空和杠杆（或通过 margin 账户）
```

**Alpaca API 核心端点：**
- `POST /v2/orders` — 下单
- `GET /v2/positions` — 持仓
- `GET /v2/account` — 余额
- `DELETE /v2/orders/{id}` — 撤单
- `GET /v2/assets/{symbol}/bars` — K线

### Phase 2: 港股交易（LongPort）— 3-5天
```
用户 → NOFXi Agent → trader/longport/ → LongPort OpenAPI
```
**为什么选长桥：**
- 官方 Go SDK（github.com/longportapp/openapi-go）
- 同时支持美股+港股
- 专为程序化交易设计
- 文档完善，中文友好

**核心实现：**
```go
// trader/longport/trader.go
type LongPortTrader struct {
    config   *longport.Config
    tradeCtx *trade.TradeContext
    quoteCtx *quote.QuoteContext
    market   string  // "US" or "HK"
}
```

**需要注意：**
- 股票交易没有"做空"概念（需要融券，门槛高）
- 没有杠杆概念（除融资融券）
- 需要改造 Trader interface，增加股票买卖方法或适配现有接口
- 委托类型：限价单/市价单/条件单

### Phase 3: A股交易（长期规划）
**推荐路径：** 通过 LongPort 或 IBKR 的沪港通/深港通通道
- 不建议直接对接国内券商 API（合规风险大）
- 先支持沪港通标的（300+ 只大盘股）
- 未来考虑接入正规量化平台（掘金/QMT）

## 四、Trader Interface 适配

当前 interface 是为**加密货币合约**设计的（做多/做空/杠杆），股票交易需要扩展：

```go
// 方案A: 扩展现有 interface（推荐）
// 股票交易映射到现有接口：
// OpenLong  → 买入（leverage=1）
// CloseLong → 卖出
// OpenShort → 融券卖出（如果支持）
// CloseShort → 买入平仓
// 杠杆固定为 1（现金账户）

// 方案B: 新增 StockTrader interface
type StockTrader interface {
    Buy(symbol string, qty float64, orderType string, limitPrice float64) (OrderResult, error)
    Sell(symbol string, qty float64, orderType string, limitPrice float64) (OrderResult, error)
    GetQuote(symbol string) (Quote, error)
    GetPositions() ([]StockPosition, error)
    GetBalance() (StockBalance, error)
    GetOrders() ([]StockOrder, error)
    CancelOrder(orderID string) error
}
```

**推荐方案A**：复用现有 interface，stock trader 内部做映射。这样 Agent 的 tool calling、Web UI、Telegram 全部零改动就能支持股票。

## 五、Agent 工具扩展

```go
// 新增 LLM 工具
"buy_stock"     → 买入股票（市价/限价）
"sell_stock"    → 卖出股票
"stock_order"   → 查看委托状态
"cancel_order"  → 撤单

// 或者复用现有 execute_trade 工具，通过 symbol 前缀区分：
// "AAPL" → Alpaca
// "0700.HK" → LongPort HK
// "600519.SH" → LongPort A股通
```

## 六、数据增强

| 数据 | 来源 | 用途 |
|------|------|------|
| 实时行情 | 新浪财经（已有） + Alpaca/LongPort WebSocket | 盘中实时价格 |
| 盘前盘后 | 新浪（已有） + Alpaca extended hours | 盘前盘后交易 |
| 财报日历 | Yahoo Finance / Alpaca | 财报预警 |
| 新闻 | Alpha Vantage / Alpaca News API | 个股新闻 |
| 基本面 | Yahoo Finance / LongPort | PE/PB/市值等 |

## 七、开发计划

### Sprint 1: Alpaca Paper Trading（P0，2-3天）
- [ ] trader/alpaca/ 基础结构
- [ ] 实现 Trader interface（Buy/Sell 映射到 OpenLong/CloseLong）
- [ ] Paper Trading 环境对接
- [ ] Agent 工具适配（识别股票 symbol 自动路由到 Alpaca）
- [ ] 前端展示股票持仓
- [ ] 集成测试：从对话到下单全流程

### Sprint 2: Alpaca Live Trading + 行情增强（P1，2天）
- [ ] Live API 对接
- [ ] 盘前盘后交易支持
- [ ] WebSocket 实时行情
- [ ] 委托类型：限价单、止损单
- [ ] 风控：单笔金额限制、日亏损限制

### Sprint 3: LongPort 港股（P2，3-5天）
- [ ] trader/longport/ 基础结构
- [ ] Go SDK 集成
- [ ] 港股交易流程
- [ ] 港股行情接入
- [ ] 港股持仓管理

### Sprint 4: A股通道（P3，探索）
- [ ] 通过 LongPort/IBKR 沪港通
- [ ] A股特殊规则（T+1、涨跌停、集合竞价）
- [ ] A股行情增强

## 八、风险与注意事项

1. **资金安全** — 所有交易必须经过确认流程，和加密货币一样
2. **合规** — 不碰灰色地带，只用正规券商 API
3. **交易规则差异** — 股票无 24h 交易，需处理休市/停牌
4. **结算周期** — 美股 T+1，港股 T+2，买入后不能立即卖出（Day Trade 规则）
5. **Pattern Day Trader** — 美股 margin 账户 5 天内 ≥4 次日内交易需 $25K 最低资金
6. **汇率** — 港股/美股涉及外汇，需要考虑汇率展示
