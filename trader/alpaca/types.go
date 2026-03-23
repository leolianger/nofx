package alpaca

import "nofx/trader/types"

// Re-export for convenience
type (
	ClosedPnLRecord = types.ClosedPnLRecord
	OpenOrder       = types.OpenOrder
)

// AlpacaAccount represents Alpaca account info
type AlpacaAccount struct {
	ID               string `json:"id"`
	AccountNumber    string `json:"account_number"`
	Status           string `json:"status"`
	Currency         string `json:"currency"`
	Cash             string `json:"cash"`
	Equity           string `json:"equity"`
	BuyingPower      string `json:"buying_power"`
	PortfolioValue   string `json:"portfolio_value"`
	PatternDayTrader bool   `json:"pattern_day_trader"`
	DaytradeCount    int    `json:"daytrade_count"`
	TradingBlocked   bool   `json:"trading_blocked"`
}

// AlpacaPosition represents an open position
type AlpacaPosition struct {
	AssetID        string `json:"asset_id"`
	Symbol         string `json:"symbol"`
	Exchange       string `json:"exchange"`
	AssetClass     string `json:"asset_class"`
	AvgEntryPrice  string `json:"avg_entry_price"`
	Qty            string `json:"qty"`
	Side           string `json:"side"`
	MarketValue    string `json:"market_value"`
	CostBasis      string `json:"cost_basis"`
	UnrealizedPL   string `json:"unrealized_pl"`
	UnrealizedPLPC string `json:"unrealized_plpc"`
	CurrentPrice   string `json:"current_price"`
	LastdayPrice   string `json:"lastday_price"`
	ChangeToday    string `json:"change_today"`
}

// AlpacaOrderRequest represents an order submission
type AlpacaOrderRequest struct {
	Symbol      string `json:"symbol"`
	Qty         string `json:"qty,omitempty"`
	Notional    string `json:"notional,omitempty"` // Dollar amount instead of qty
	Side        string `json:"side"`               // buy, sell
	Type        string `json:"type"`               // market, limit, stop, stop_limit
	TimeInForce string `json:"time_in_force"`      // day, gtc, opg, cls, ioc, fok
	LimitPrice  string `json:"limit_price,omitempty"`
	StopPrice   string `json:"stop_price,omitempty"`
}

// AlpacaOrder represents an order response
type AlpacaOrder struct {
	ID             string `json:"id"`
	ClientOrderID  string `json:"client_order_id"`
	Symbol         string `json:"symbol"`
	Side           string `json:"side"`
	Type           string `json:"type"`
	Qty            string `json:"qty"`
	FilledQty      string `json:"filled_qty"`
	FilledAvgPrice string `json:"filled_avg_price"`
	LimitPrice     string `json:"limit_price"`
	StopPrice      string `json:"stop_price"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	SubmittedAt    string `json:"submitted_at"`
	FilledAt       string `json:"filled_at"`
	TimeInForce    string `json:"time_in_force"`
}
