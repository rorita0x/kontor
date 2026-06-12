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

// Color liefert die zum Ergebnis passende Bootstrap-Kontextfarbe für Badges.
func (r TradeResult) Color() string {
	switch r {
	case RESULT_WIN:
		return "success"
	case RESULT_LOSS:
		return "danger"
	case RESULT_BREAKEVEN:
		return "warning"
	case RESULT_NOTFINISHED:
		return "info"
	default:
		return "secondary"
	}
}

// Display liefert das deutschsprachige Label für die Anzeige.
func (r TradeResult) Display() string {
	switch r {
	case RESULT_WIN:
		return "Gewinn"
	case RESULT_LOSS:
		return "Verlust"
	case RESULT_BREAKEVEN:
		return "Break-Even"
	case RESULT_NOTFINISHED:
		return "Offen"
	case RESULT_SKIP:
		return "Skip"
	default:
		return string(r)
	}
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
	Leverage      F32    `form:"leverage" json:"leverage"` // Hebel (z. B. 10 = 10x)
	Margin        F32    `form:"margin" json:"margin"`     // gebundene Margin in Kontowährung
	Exits         []Exit `json:"exits"`

	// Settled ist der Teil des realisierten Ergebnisses dieses Trades, der bereits
	// mit dem gespeicherten Kontostand (Trading-Kapital) verrechnet wurde. So
	// erkennt die App, was noch offen zu verrechnen ist, und stupst nicht doppelt.
	Settled F32 `form:"settled" json:"settled"`
}

// RealizedPnL ist das realisierte Ergebnis dieses Trades in Kontowährung,
// berechnet aus den Exits: je Exit (Exit-Preis − Entry) × anteilige Stückzahl,
// richtungsabhängig (bei Short umgekehrt). Die Exit-Menge wird als Prozent der
// Position interpretiert. Dient nur als Vorschlag fürs Verrechnen.
func (t Trade) RealizedPnL() F32 {
	qty := t.EffectiveQty()
	if qty <= 0 || t.EntryPrice <= 0 {
		return 0
	}
	sign := F32(1)
	if t.Traded == TRADED_SHORT {
		sign = -1
	}
	var pnl F32
	for _, e := range t.Exits {
		if e.Price <= 0 {
			continue
		}
		part := qty * e.Quantity / 100
		pnl += (e.Price - t.EntryPrice) * part * sign
	}
	return pnl
}

// UnsettledPnL ist der noch nicht mit dem Kontostand verrechnete Teil des
// realisierten Ergebnisses. ~0 bedeutet: nichts offen.
func (t Trade) UnsettledPnL() F32 {
	return t.RealizedPnL() - t.Settled
}

// NeedsReconcile meldet, ob ein nennenswerter Betrag offen ist (≥ 1 Cent).
func (t Trade) NeedsReconcile() bool {
	d := t.UnsettledPnL()
	if d < 0 {
		d = -d
	}
	return d >= 0.01
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
	Title     string `storm:"id"`
	RiskLimit F32    `form:"riskLimit" json:"riskLimit"` // erlaubtes offenes Risiko in % vom Konto (nur informativ)
}

type ClassRiskSummary struct {
	Class           string
	TotalRisk       F32 // Summe der Konto-Risiko-Prozente
	TotalRiskAmount F32 // Summe des Risikos in Kontowährung
	TradeCount      int
	HasLimit        bool // ob für die Klasse ein Limit gepflegt ist
	LimitPct        F32  // Limit in % vom Konto
	LimitAmount     F32  // Limit in Kontowährung (LimitPct * Konto / 100)
	FreePct         F32  // noch freies Risiko in % (LimitPct - TotalRisk)
	FreeAmount      F32  // noch freies Risiko in Kontowährung
}

// SymbolRiskSummary fasst das offene Risiko eines einzelnen Symbols gegen das
// globale Limit pro Asset zusammen (für die Übersicht und das Eintrags-Formular).
type SymbolRiskSummary struct {
	Symbol          string
	Class           string
	TotalRisk       F32
	TotalRiskAmount F32
	TradeCount      int
	HasLimit        bool
	LimitPct        F32
	LimitAmount     F32
	FreePct         F32 // effektiv freies Risiko = min(Asset-frei, Sektor-frei)
	FreeAmount      F32
	SectorBinds     bool // true, wenn das Sektor-Limit die kleinere (bindende) Grenze ist
}

// StatsSummary fasst Kennzahlen über eine Menge von Trades zusammen
// (für die Kennzahlen-Leiste auf der Übersicht).
type StatsSummary struct {
	Total       int
	Wins        int
	Losses      int
	BreakEven   int
	Open        int // not finished
	Skips       int
	WinRate     F32 // % der entschiedenen Trades (Win / (Win+Loss))
	OpenRisk    F32 // offenes Risiko in Kontowährung
	OpenRiskPct F32 // offenes Risiko in % vom Konto
}

// Settings hält globale Einstellungen. Es gibt genau einen Datensatz mit Pk = 1.
type Settings struct {
	Pk                int `storm:"id" form:"-"`
	AccountSize       F32 `form:"accountSize" json:"accountSize"`
	PerAssetRiskLimit F32 `form:"perAssetRiskLimit" json:"perAssetRiskLimit"` // generelles Limit pro Asset in % vom Konto (nur informativ)
}
