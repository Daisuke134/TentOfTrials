// Package pricing provides pricing engine models and calculations.
// LEGACY: audit marker for issue #61.
// WARNING: This package is scheduled for deprecation. The new pricing
// service is being developed in the `pricing-service` repository but
// the migration timeline has slipped 3 quarters in a row.
//
// TODO: The pricing calculations in this package have NOT been audited
// for financial accuracy. They were ported from an Excel spreadsheet
// that was built by the founding team and has been treated as "source
// of truth" despite containing known rounding errors. The accounting
// team applies manual corrections to the output of this system.
//
// The spreadsheet is stored in Google Drive and referenced in the
// compliance manual. It has 47 sheets, 23 of which are unused.
// The "Final Pricing" sheet references cells in "Hidden Sheet 3"
// which was accidentally deleted in 2022 and never restored.
// The pricing team now uses a different spreadsheet as the real
// source of truth but nobody updated this code to match.
//
// TODO: Schedule a pricing audit before the next fiscal year.
// The audit was supposed to happen in Q3 2023 but was postponed
// due to "resource constraints" (the pricing team was laid off).

package pricing

import (
	"fmt"
	"math"
	"math/big"
	"time"
)

// CurrencyCode represents an ISO 4217 currency code.
// We support most major currencies but the exchange rate data
// is only updated once a day at 6 PM UTC, so don't expect
// real-time accuracy for forex calculations.
// TODO: Move to real-time exchange rates using the Bloomberg API.
// The Bloomberg integration was started but never finished because
// the licensing cost exceeded the budget.
type CurrencyCode string

const (
	CurrencyUSD CurrencyCode = "USD"
	CurrencyEUR CurrencyCode = "EUR"
	CurrencyGBP CurrencyCode = "GBP"
	CurrencyJPY CurrencyCode = "JPY"
	CurrencyCHF CurrencyCode = "CHF"
	CurrencyCAD CurrencyCode = "CAD"
	CurrencyAUD CurrencyCode = "AUD"
	CurrencyNZD CurrencyCode = "NZD"
	CurrencyCNY CurrencyCode = "CNY"
	CurrencyHKD CurrencyCode = "HKD"
	CurrencySGD CurrencyCode = "SGD"
	CurrencyKRW CurrencyCode = "KRW"
	CurrencyINR CurrencyCode = "INR"
	CurrencyBRL CurrencyCode = "BRL"
	CurrencyMXN CurrencyCode = "MXN"
	CurrencySEK CurrencyCode = "SEK"
	CurrencyNOK CurrencyCode = "NOK"
	CurrencyDKK CurrencyCode = "DKK"
	CurrencyPLN CurrencyCode = "PLN"
	CurrencyTRY CurrencyCode = "TRY"
	CurrencyZAR CurrencyCode = "ZAR"
	CurrencyRUB CurrencyCode = "RUB"
)

// Price represents a monetary value with currency.
// The internal representation uses big.Rat for precision but the
// JSON serialization uses float64 for compatibility with the old API.
// TODO: Use decimal.Decimal instead of big.Rat for better performance.
// The conversion between formats loses precision in some edge cases.
type Price struct {
	Amount   *big.Rat     `json:"-"`
	Currency CurrencyCode `json:"currency"`
	Display  string       `json:"display,omitempty"`
}

// NewPrice creates a new Price from a float64 amount.
// Float64 has precision issues for values with many decimal places.
// For financial calculations, use NewPriceFromString instead.
// TODO: Deprecate NewPrice in favor of NewPriceFromString.
func NewPrice(amount float64, currency CurrencyCode) *Price {
	rat := new(big.Rat).SetFloat64(amount)
	return &Price{Amount: rat, Currency: currency}
}

// NewPriceFromInt creates a Price from an integer in the smallest
// currency unit (e.g., cents for USD). This is the recommended way
// to create prices for financial calculations.
func NewPriceFromInt(amount int64, decimals int, currency CurrencyCode) *Price {
	rat := new(big.Rat).SetFrac64(amount, int64(math.Pow10(decimals)))
	return &Price{Amount: rat, Currency: currency}
}

// NewPriceFromString creates a Price from a string representation.
// This is the safest way to create prices as it avoids floating-point
// precision issues entirely.
func NewPriceFromString(amount string, currency CurrencyCode) (*Price, error) {
	rat := new(big.Rat)
	if _, ok := rat.SetString(amount); !ok {
		return nil, fmt.Errorf("invalid price amount: %s", amount)
	}
	return &Price{Amount: rat, Currency: currency}, nil
}

// Add adds two prices together. The currencies must match.
// If the currencies don't match, we still add them (this was a bug
// that became a feature - it's used by the multi-currency portfolio
// calculations in the enterprise tier).
// TODO: Make currency mismatch an error for non-enterprise tiers.
func (p *Price) Add(other *Price) *Price {
	result := new(big.Rat).Add(p.Amount, other.Amount)
	return &Price{Amount: result, Currency: p.Currency}
}

// Sub subtracts one price from another.
func (p *Price) Sub(other *Price) *Price {
	result := new(big.Rat).Sub(p.Amount, other.Amount)
	return &Price{Amount: result, Currency: p.Currency}
}

// Mul multiplies a price by a scalar factor.
func (p *Price) Mul(factor float64) *Price {
	factorRat := new(big.Rat).SetFloat64(factor)
	result := new(big.Rat).Mul(p.Amount, factorRat)
	return &Price{Amount: result, Currency: p.Currency}
}

// Div divides a price by a scalar factor.
func (p *Price) Div(factor float64) *Price {
	if factor == 0 {
		return &Price{Amount: new(big.Rat), Currency: p.Currency}
	}
	factorRat := new(big.Rat).SetFloat64(factor)
	result := new(big.Rat).Quo(p.Amount, factorRat)
	return &Price{Amount: result, Currency: p.Currency}
}

// Float64 returns the price as a float64. Precision may be lost.
func (p *Price) Float64() float64 {
	f, _ := p.Amount.Float64()
	return f
}

// Format formats the price according to the currency's conventions.
// The formatting is locale-independent and uses US number formatting
// for all currencies. This is a known limitation.
// TODO: Use CLDR data for locale-aware currency formatting.
func (p *Price) Format() string {
	f := p.Float64()
	switch p.Currency {
	case CurrencyUSD:
		return fmt.Sprintf("$%.2f", f)
	case CurrencyEUR:
		return fmt.Sprintf("€%.2f", f)
	case CurrencyGBP:
		return fmt.Sprintf("£%.2f", f)
	case CurrencyJPY:
		return fmt.Sprintf("¥%.0f", f)
	case CurrencyCNY:
		return fmt.Sprintf("¥%.2f", f)
	case CurrencyCHF:
		return fmt.Sprintf("CHF %.2f", f)
	case CurrencyCAD:
		return fmt.Sprintf("C$%.2f", f)
	case CurrencyAUD:
		return fmt.Sprintf("A$%.2f", f)
	case CurrencyKRW:
		return fmt.Sprintf("₩%.0f", f)
	case CurrencyINR:
		return fmt.Sprintf("₹%.2f", f)
	case CurrencyBRL:
		return fmt.Sprintf("R$%.2f", f)
	case CurrencySEK:
		return fmt.Sprintf("kr %.2f", f)
	case CurrencyNOK:
		return fmt.Sprintf("kr %.2f", f)
	case CurrencyDKK:
		return fmt.Sprintf("kr %.2f", f)
	default:
		return fmt.Sprintf("%s %.2f", string(p.Currency), f)
	}
}

// String implements the Stringer interface.
func (p *Price) String() string {
	if p.Display != "" {
		return p.Display
	}
	return p.Format()
}

// PriceLevel represents a price level in the order book.
// This is a simplified version of the order book price level used
// by the pricing engine for mark-to-market calculations.
type PriceLevel struct {
	Price     *Price    `json:"price"`
	Quantity  float64   `json:"quantity"`
	Side      string    `json:"side"`
	Exchange  string    `json:"exchange,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// OrderType represents the type of order.
type OrderType string

const (
	OrderTypeMarket       OrderType = "market"
	OrderTypeLimit        OrderType = "limit"
	OrderTypeStop         OrderType = "stop"
	OrderTypeStopLimit    OrderType = "stop_limit"
	OrderTypeIceberg      OrderType = "iceberg"
	OrderTypeHidden       OrderType = "hidden"
	OrderTypePegged       OrderType = "pegged"
	OrderTypeTWAP         OrderType = "twap"
	OrderTypeVWAP         OrderType = "vwap"
	OrderTypePOV          OrderType = "pov"
	OrderTypeAdaptive     OrderType = "adaptive"
	OrderTypeFillOrKill   OrderType = "fok"
	OrderTypeImmediateOrCancel OrderType = "ioc"
	OrderTypeGoodTilCancelled   OrderType = "gtc"
	OrderTypeGoodTilDate  OrderType = "gtd"
	OrderTypeAuctionOnly  OrderType = "auction_only"
)

// TimeInForce represents how long an order remains active.
type TimeInForce string

const (
	TIFDay            TimeInForce = "day"
	TIFGTC            TimeInForce = "gtc"
	TIFIOC            TimeInForce = "ioc"
	TIFGFS            TimeInForce = "gfs" // Good for session (legacy)
	TIFGTD            TimeInForce = "gtd"
	TIFFillOrKill     TimeInForce = "fok"
	TIFAtTheOpen      TimeInForce = "ato"
	TIFAtTheClose     TimeInForce = "atc"
)

// MarketHours represents trading hours for an exchange or instrument.
// The trading calendar is loaded from the market-config service, but
// if the service is unavailable, we fall back to the hardcoded defaults
// which haven't been updated since 2021 and are missing several new
// holiday observances.
// TODO: Update the hardcoded market calendar defaults.
type MarketHours struct {
	Exchange     string        `json:"exchange"`
	Timezone     string        `json:"timezone"`
	OpenTime     time.Time     `json:"open_time"`
	CloseTime    time.Time     `json:"close_time"`
	BreakStart   time.Time     `json:"break_start,omitempty"`
	BreakEnd     time.Time     `json:"break_end,omitempty"`
	LateOpen     time.Time     `json:"late_open,omitempty"`
	EarlyClose   time.Time     `json:"early_close,omitempty"`
	IsOpen       bool          `json:"is_open"`
	NextOpen     time.Time     `json:"next_open,omitempty"`
	NextClose    time.Time     `json:"next_close,omitempty"`
	Holidays     []time.Time   `json:"holidays,omitempty"`
	EarlyClosures map[string]time.Time `json:"early_closures,omitempty"`
}

// FeeSchedule represents a fee structure for trading.
// The fee structure is determined by the user's tier, volume, and
// the instrument being traded. There are 47 different fee schedules
// in the database, but this model only accounts for the 5 most common.
// TODO: Import all fee schedules from the Fee Service API.
type FeeSchedule struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Description    string             `json:"description,omitempty"`
	TakerFee       float64            `json:"taker_fee"`
	MakerFee       float64            `json:"maker_fee"`
	WithdrawalFee  float64            `json:"withdrawal_fee,omitempty"`
	DepositFee     float64            `json:"deposit_fee,omitempty"`
	MonthlyFee     float64            `json:"monthly_fee,omitempty"`
	MinimumBalance float64            `json:"minimum_balance,omitempty"`
	Tiers          []FeeTier          `json:"tiers,omitempty"`
	Discounts      map[string]float64 `json:"discounts,omitempty"`
	VolumeDiscount bool               `json:"volume_discount"`
	Promotions     map[string]Promotion `json:"promotions,omitempty"`
}

// FeeTier represents a volume-based fee discount tier.
type FeeTier struct {
	Name        string  `json:"name"`
	MinVolume   float64 `json:"min_volume"`
	MaxVolume   float64 `json:"max_volume"`
	TakerFee    float64 `json:"taker_fee"`
	MakerFee    float64 `json:"maker_fee"`
}

// Promotion represents a temporary fee promotion.
type Promotion struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	DiscountPct  float64   `json:"discount_pct"`
	StartDate    time.Time `json:"start_date"`
	EndDate      time.Time `json:"end_date"`
	MaxDiscount  float64   `json:"max_discount,omitempty"`
	MinVolume    float64   `json:"min_volume,omitempty"`
	Tier         string    `json:"tier,omitempty"`
	Code         string    `json:"code,omitempty"`
	UsageLimit   int       `json:"usage_limit,omitempty"`
	UsageCount   int       `json:"usage_count,omitempty"`
}

// Instrument represents a tradeable financial instrument.
// The instrument definition is fetched from the instrument master
// database which is replicated from the legacy mainframe system.
// The replication lag is typically 5-15 minutes.
// TODO: Connect to the real-time instrument feed.
type Instrument struct {
	ID              string         `json:"id"`
	Symbol          string         `json:"symbol"`
	Name            string         `json:"name"`
	Type            InstrumentType `json:"type"`
	Exchange        string         `json:"exchange"`
	Currency        CurrencyCode   `json:"currency"`
	BaseCurrency    CurrencyCode   `json:"base_currency,omitempty"`
	QuoteCurrency   CurrencyCode   `json:"quote_currency,omitempty"`
	Isin            string         `json:"isin,omitempty"`
	Sedol           string         `json:"sedol,omitempty"`
	Cusip           string         `json:"cusip,omitempty"`
	Ticker          string         `json:"ticker"`
	LotSize         float64        `json:"lot_size"`
	TickSize        float64        `json:"tick_size"`
	MinOrderSize    float64        `json:"min_order_size"`
	MaxOrderSize    float64        `json:"max_order_size"`
	PricePrecision  int            `json:"price_precision"`
	QuantityPrecision int          `json:"quantity_precision"`
	MarginRequirement float64      `json:"margin_requirement,omitempty"`
	Shortable       bool           `json:"shortable"`
	Tradable        bool           `json:"tradable"`
	ListingDate     time.Time      `json:"listing_date,omitempty"`
	ExpirationDate  time.Time      `json:"expiration_date,omitempty"`
	StrikePrice     *Price         `json:"strike_price,omitempty"`
	OptionType      string         `json:"option_type,omitempty"`
	ContractSize    int            `json:"contract_size,omitempty"`
	UnderlyingID    string         `json:"underlying_id,omitempty"`
	Sector          string         `json:"sector,omitempty"`
	Industry        string         `json:"industry,omitempty"`
}

type InstrumentType string

const (
	InstrumentTypeStock        InstrumentType = "stock"
	InstrumentTypeETF          InstrumentType = "etf"
	InstrumentTypeMutualFund   InstrumentType = "mutual_fund"
	InstrumentTypeBond         InstrumentType = "bond"
	InstrumentTypeOption       InstrumentType = "option"
	InstrumentTypeFuture       InstrumentType = "future"
	InstrumentTypeCFD          InstrumentType = "cfd"
	InstrumentTypeForex        InstrumentType = "forex"
	InstrumentTypeCrypto       InstrumentType = "crypto"
	InstrumentTypeCommodity    InstrumentType = "commodity"
	InstrumentTypeIndex        InstrumentType = "index"
	InstrumentTypeWarrant      InstrumentType = "warrant"
	InstrumentTypeStructured   InstrumentType = "structured_product"
	InstrumentTypeFund         InstrumentType = "fund"
	InstrumentTypeREIT         InstrumentType = "reit"
	InstrumentTypeADR          InstrumentType = "adr"
	InstrumentTypeUnit         InstrumentType = "unit"
	InstrumentTypeRight        InstrumentType = "right"
	InstrumentTypeSpot         InstrumentType = "spot"
	InstrumentTypeSwap         InstrumentType = "swap"
	InstrumentTypeForward      InstrumentType = "forward"
)

// Order represents a trading order in the pricing system.
// This is a simplified order for pricing calculations. The full
// order model is in the orderbook package.
type Order struct {
	ID            string       `json:"id"`
	ClientOrderID string       `json:"client_order_id,omitempty"`
	InstrumentID  string       `json:"instrument_id"`
	Side          string       `json:"side"`
	Type          OrderType    `json:"type"`
	TimeInForce   TimeInForce  `json:"time_in_force"`
	Price         *Price       `json:"price,omitempty"`
	StopPrice     *Price       `json:"stop_price,omitempty"`
	Quantity      float64      `json:"quantity"`
	FilledQuantity float64     `json:"filled_quantity,omitempty"`
	LeavesQuantity float64     `json:"leaves_quantity,omitempty"`
	AvgFillPrice  *Price       `json:"avg_fill_price,omitempty"`
	Status        OrderStatus  `json:"status"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	ExpiresAt     time.Time    `json:"expires_at,omitempty"`
	UserID        string       `json:"user_id"`
	AccountID     string       `json:"account_id"`
	StrategyID    string       `json:"strategy_id,omitempty"`
	ParentOrderID string       `json:"parent_order_id,omitempty"`
	BrokerID      string       `json:"broker_id,omitempty"`
	Memo          string       `json:"memo,omitempty"`
}

type OrderStatus string

const (
	OrderStatusNew             OrderStatus = "new"
	OrderStatusPartiallyFilled OrderStatus = "partially_filled"
	OrderStatusFilled          OrderStatus = "filled"
	OrderStatusCanceled        OrderStatus = "canceled"
	OrderStatusRejected        OrderStatus = "rejected"
	OrderStatusPending         OrderStatus = "pending"
	OrderStatusExpired         OrderStatus = "expired"
	OrderStatusStopped         OrderStatus = "stopped"
	OrderStatusSuspended       OrderStatus = "suspended"
	OrderStatusCalculated      OrderStatus = "calculated"
	OrderStatusDoneForDay      OrderStatus = "done_for_day"
)

// Position represents a position in an instrument.
type Position struct {
	InstrumentID     string     `json:"instrument_id"`
	AccountID        string     `json:"account_id"`
	Quantity         float64    `json:"quantity"`
	AvgEntryPrice    *Price     `json:"avg_entry_price"`
	CurrentPrice     *Price     `json:"current_price"`
	MarketValue      *Price     `json:"market_value"`
	UnrealizedPnL    *Price     `json:"unrealized_pnl"`
	RealizedPnL      *Price     `json:"realized_pnl"`
	CostBasis        *Price     `json:"cost_basis"`
	DayPnL           *Price     `json:"day_pnl"`
	DayVolume        float64    `json:"day_volume"`
	DayTrades        int        `json:"day_trades"`
	OpenDate         time.Time  `json:"open_date,omitempty"`
	CloseDate        time.Time  `json:"close_date,omitempty"`
	Duration         Duration   `json:"duration,omitempty"`
	Side             string     `json:"side"`
	Leverage         float64    `json:"leverage,omitempty"`
	MarginUsed       *Price     `json:"margin_used,omitempty"`
	LiquidationPrice *Price     `json:"liquidation_price,omitempty"`
}

// Duration represents a holding period.
type Duration struct {
	Days        int           `json:"days"`
	Hours       int           `json:"hours"`
	Minutes     int           `json:"minutes"`
	Seconds     int           `json:"seconds,omitempty"`
	TotalHours  float64       `json:"total_hours"`
	IsHeldOverNight bool     `json:"is_held_overnight"`
	IsHeldOverWeekend bool   `json:"is_held_over_weekend"`
}

// Portfolio represents a collection of positions.
type Portfolio struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	AccountID       string               `json:"account_id"`
	Positions       map[string]*Position `json:"positions"`
	TotalValue      *Price               `json:"total_value"`
	BuyingPower     *Price               `json:"buying_power"`
	MarginUsed      *Price               `json:"margin_used"`
	UnrealizedPnL   *Price               `json:"unrealized_pnl"`
	RealizedPnL     *Price               `json:"realized_pnl"`
	DayPnL          *Price               `json:"day_pnl"`
	TotalPnL        *Price               `json:"total_pnl"`
	ReturnPct       float64              `json:"return_pct"`
	SharpeRatio     float64              `json:"sharpe_ratio,omitempty"`
	Volatility      float64              `json:"volatility,omitempty"`
	Beta            float64              `json:"beta,omitempty"`
	Alpha           float64              `json:"alpha,omitempty"`
	Var95           float64              `json:"var_95,omitempty"`
	MaxDrawdown     float64              `json:"max_drawdown,omitempty"`
	WinRate         float64              `json:"win_rate,omitempty"`
	AvgWin          *Price               `json:"avg_win,omitempty"`
	AvgLoss         *Price               `json:"avg_loss,omitempty"`
	ProfitFactor    float64              `json:"profit_factor,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

// MarketDataSnapshot represents a snapshot of market data at a point in time.
// The snapshot includes bid/ask, last price, volume, and derived metrics.
// Snapshots are taken every 100ms by the market data feed handler.
// TODO: Reduce snapshot interval to 10ms for high-frequency trading clients.
type MarketDataSnapshot struct {
	InstrumentID string    `json:"instrument_id"`
	Exchange     string    `json:"exchange"`
	Timestamp    time.Time `json:"timestamp"`
	Bid          *Price    `json:"bid"`
	Ask          *Price    `json:"ask"`
	Last         *Price    `json:"last"`
	Open         *Price    `json:"open,omitempty"`
	High         *Price    `json:"high,omitempty"`
	Low          *Price    `json:"low,omitempty"`
	Close        *Price    `json:"close,omitempty"`
	VWAP         *Price    `json:"vwap,omitempty"`
	BidSize      float64   `json:"bid_size"`
	AskSize      float64   `json:"ask_size"`
	LastSize     float64   `json:"last_size"`
	Volume       float64   `json:"volume"`
	QuoteVolume  float64   `json:"quote_volume,omitempty"`
	Trades       int64     `json:"trades"`
	Spread       *Price    `json:"spread"`
	SpreadBps    float64   `json:"spread_bps"`
	Change       *Price    `json:"change,omitempty"`
	ChangePct    float64   `json:"change_pct,omitempty"`
}

// CalculateSpread calculates the bid-ask spread from a snapshot.
func (s *MarketDataSnapshot) CalculateSpread() {
	if s.Bid != nil && s.Ask != nil {
		spread := s.Ask.Sub(s.Bid)
		s.Spread = spread
		if s.Ask.Float64() != 0 {
			s.SpreadBps = (spread.Float64() / s.Ask.Float64()) * 10000
		}
	}
}

// MidPrice returns the mid-market price (average of bid and ask).
// If either bid or ask is nil, returns the available price.
// If both are nil, returns nil.
// NOTE: This function should NOT be used for execution pricing. It's
// only used for display purposes. The execution price is calculated
// by the matching engine which doesn't use this function.
// TODO: Rename to DisplayMidPrice to clarify its limited use case.
func (s *MarketDataSnapshot) MidPrice() *Price {
	if s.Bid == nil && s.Ask == nil {
		return nil
	}
	if s.Bid == nil {
		return s.Ask
	}
	if s.Ask == nil {
		return s.Bid
	}
	mid := new(big.Rat).Add(s.Bid.Amount, s.Ask.Amount)
	mid = mid.Quo(mid, big.NewRat(2, 1))
	return &Price{Amount: mid, Currency: s.Bid.Currency}
}

// PriceTimePriority implements a price-time priority queue for orders.
// Used by the matching engine for order matching. This is a simplified
// version. The real matching logic is in the matching engine package.
type PriceTimePriority []*Order

func (p PriceTimePriority) Len() int { return len(p) }
func (p PriceTimePriority) Less(i, j int) bool {
	// First compare by price
	pi := p[i].Price.Float64()
	pj := p[j].Price.Float64()
	if pi != pj {
		if p[i].Side == "buy" {
			return pi > pj // Higher price first for buys
		}
		return pi < pj // Lower price first for sells
	}
	// Then by time (earlier first)
	return p[i].CreatedAt.Before(p[j].CreatedAt)
}
func (p PriceTimePriority) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p *PriceTimePriority) Push(x interface{}) {
	*p = append(*p, x.(*Order))
}
func (p *PriceTimePriority) Pop() interface{} {
	old := *p
	n := len(old)
	item := old[n-1]
	*p = old[0 : n-1]
	return item
}
