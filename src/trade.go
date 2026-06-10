package src

import (
	"strconv"
	"time"
)

type Exit struct {
	Date     string `json:"date"`
	Price    F32    `json:"price"`
	Quantity F32    `json:"qty"`
}

type Traded string

type TradeResult string

const (
	TRADED_SKIP  Traded = "skip"
	TRADED_LONG         = "long"
	TRADED_SHORT        = "short"
)

const (
	RESULT_NOTFINISHED TradeResult = "not finished"
	RESULT_SKIP                    = "skip"
	RESULT_LOSS                    = "loss"
	RESULT_BREAKEVEN               = "break-even"
	RESULT_WIN                     = "win"
)

func (r TradeResult) IsCorrect() bool {
	return r == RESULT_WIN
}

type F32 float32

func (f *F32) UnmarshalText(b []byte) error {
	v, err := strconv.ParseFloat(string(b), 32)
	if err != nil {
		return err
	}
	*f = F32(v)
	return nil
}

func (f F32) MarshalText() ([]byte, error) {
	s := strconv.FormatFloat(float64(f), 'f', 2, 32)
	return []byte(s), nil
}

func (f F32) String() string {
	return strconv.FormatFloat(float64(f), 'f', 2, 32)
}

type Trade struct {
	Pk        int       `storm:"id,increment" json:"id" form:"id" uri:"id"`
	CreatedAt time.Time `storm:"index" json:"createdAt" form:"-" uri:"-" binding:"-"`

	Symbol      string   `form:"symbol" json:"symbol" form:"symbol" binding:"required"`
	Screenshots []string `form:"screenshots[]" json:"screenshots"`
	Tags        []string `form:"tags[]" json:"tags"`

	Notes string `form:"notes" json:"notes"`

	Traded Traded      `form:"traded" binding:"required"`
	Result TradeResult `form:"result" binding:"required"`

	EntryPrice    F32    `form:"entryPrice" json:"entryPrice"`
	StopLoss      F32    `form:"stopLoss" json:"stopLoss"`
	Quantity      F32    `form:"quantity" json:"quantity"`
	PositionValue F32    `form:"positionValue" json:"positionValue"`
	Exits         []Exit `json:"exits"`
}

// signedDiff ist der richtungsabhängige Kursabstand vom Entry zum Stop-Loss.
// Positiv = Stop liegt in Verlustrichtung (echtes Risiko); negativ = Stop liegt
// in Gewinnrichtung (gesicherter Gewinn, kein Risiko). Bei Short ist die
// Verlustrichtung nach oben, bei Long (und sonst) nach unten.
func (t Trade) signedDiff() F32 {
	if t.Traded == TRADED_SHORT {
		return t.StopLoss - t.EntryPrice
	}
	return t.EntryPrice - t.StopLoss
}

// RiskFromSL liefert den richtungsabhängigen Kursabstand vom Entry zum Stop-Loss
// in Prozent. Das ist NICHT das Konto-Risiko, sondern nur der SL-Abstand.
// Negativ, wenn der Stop bereits im Gewinn liegt.
func (t Trade) RiskFromSL() F32 {
	if t.EntryPrice <= 0 || t.StopLoss <= 0 {
		return 0
	}
	return t.signedDiff() / t.EntryPrice * 100
}

// EffectiveQty vereinheitlicht die beiden Eingabewege: bevorzugt die direkt
// eingegebene Stückzahl, sonst wird sie aus dem Positionswert und dem Entry abgeleitet.
func (t Trade) EffectiveQty() F32 {
	if t.Quantity > 0 {
		return t.Quantity
	}
	if t.PositionValue > 0 && t.EntryPrice > 0 {
		return t.PositionValue / t.EntryPrice
	}
	return 0
}

// RiskAmount ist das tatsächliche Risiko in Kontowährung: Stückzahl × Kursabstand,
// richtungsabhängig. Negativ, wenn der Stop bereits im Gewinn liegt (kein Risiko).
func (t Trade) RiskAmount() F32 {
	qty := t.EffectiveQty()
	if qty <= 0 || t.EntryPrice <= 0 || t.StopLoss <= 0 {
		return 0
	}
	return qty * t.signedDiff()
}

// RiskPercent ist das Risiko als Anteil der Kontogröße in Prozent.
func (t Trade) RiskPercent(accountSize F32) F32 {
	if accountSize <= 0 {
		return 0
	}
	return t.RiskAmount() / accountSize * 100
}

type Tag struct {
	Title string `storm:"id"`
}

type Asset struct {
	Symbol     string `storm:"id"`
	AssetClass string `form:"assetClass" json:"assetClass"`
}

type AssetClass struct {
	Title string `storm:"id"`
}

type ClassRiskSummary struct {
	Class           string
	TotalRisk       F32 // Summe der Konto-Risiko-Prozente
	TotalRiskAmount F32 // Summe des Risikos in Kontowährung
	TradeCount      int
}

// Settings hält globale Einstellungen. Es gibt genau einen Datensatz mit Pk = 1.
type Settings struct {
	Pk          int `storm:"id" form:"-"`
	AccountSize F32 `form:"accountSize" json:"accountSize"`
}
