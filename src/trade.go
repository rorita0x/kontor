package src

import (
	"strconv"
	"strings"
	"time"
)

type Exit struct {
	Date     string `json:"date"`
	Price    F32    `json:"price"`
	Quantity F32    `json:"qty"`

	// Settled hält fest, ob dieser Exit bereits mit dem Kontostand verrechnet
	// wurde. SettledAmount ist der dabei tatsächlich gebuchte Betrag (kann vom
	// rechnerischen Ergebnis abweichen, z. B. wegen Gebühren oder verteiltem Geld).
	Settled       bool `json:"settled"`
	SettledAmount F32  `json:"settledAmount"`

	// FxRate ist der Wechselkurs Kontowährung je Trade-Währung zum Zeitpunkt
	// dieses Exits (beim Swing-Trade oft anders als bei Eröffnung). Dient nur dem
	// EUR-Vorschlag der Verrechnung. 0/leer = Trade-Kurs verwenden.
	FxRate FxRate `json:"fxRate"`
}

// FxRate ist ein Wechselkurs mit voller Genauigkeit. Anders als F32 rundet er
// nicht auf 2 Nachkommastellen (für einen Kurs wie 0,8638 essenziell) und liest
// einen leeren String als 0 (analog zu den übrigen optionalen Formularfeldern).
type FxRate float64

func (f *FxRate) UnmarshalText(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = FxRate(v)
	return nil
}

func (f FxRate) MarshalText() ([]byte, error) {
	return []byte(strconv.FormatFloat(float64(f), 'f', -1, 64)), nil
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
	// Leeres Feld (z. B. ein noch nicht ausgefülltes Exit-Betragsfeld) als 0 lesen,
	// damit ein einzelnes leeres Feld nicht das Dekodieren der ganzen Exit-Liste
	// abbricht (encoding/json bräche bei ParseFloat-Fehler ab).
	s := strings.TrimSpace(string(b))
	if s == "" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 32)
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
	Margin        F32    `form:"margin" json:"margin"`     // gebundene Margin in Trade-Währung
	Exits         []Exit `json:"exits"`

	// FxRate ist der Wechselkurs Kontowährung je Trade-Währung bei Eröffnung
	// (z. B. EUR je USD ≈ 0,86). Alle Geldbeträge des Trades (Entry, Stop, Margin,
	// Positionswert, Risiko) sind in Trade-Währung notiert; FxRate rechnet sie für
	// die Konto-Sicht (Risiko-%, Dashboard, Verrechnung) auf die Kontowährung um.
	// 0/leer = 1, d. h. der Trade läuft direkt in Kontowährung (keine Umrechnung).
	// float64 statt F32, damit der Kurs nicht auf 2 Nachkommastellen gerundet wird.
	FxRate float64 `form:"fxRate" json:"fxRate"`
}

// ExitPnL ist das rechnerische Ergebnis eines einzelnen Exits in Kontowährung:
// (Exit-Preis − Entry) × anteilige Stückzahl, richtungsabhängig (bei Short
// umgekehrt). Die Exit-Menge wird als Prozent der Position interpretiert. Dient
// nur als Vorschlag für den zu verrechnenden Betrag.
func (t Trade) ExitPnL(e Exit) F32 {
	qty := t.EffectiveQty()
	if qty <= 0 || t.EntryPrice <= 0 || e.Price <= 0 {
		return 0
	}
	sign := F32(1)
	if t.Traded == TRADED_SHORT {
		sign = -1
	}
	return (e.Price - t.EntryPrice) * (qty * e.Quantity / 100) * sign
}

// SettledTotal ist die Summe der tatsächlich verrechneten Beträge aller bereits
// als verrechnet markierten Exits. Aus der Differenz zweier Stände (alt/neu)
// ergibt sich, wie stark der Kontostand beim Speichern anzupassen ist.
func (t Trade) SettledTotal() F32 {
	var sum F32
	for _, e := range t.Exits {
		if e.Settled {
			sum += e.SettledAmount
		}
	}
	return sum
}

// UnsettledExitsPnL ist das rechnerische Ergebnis aller Exits, die noch nicht
// verrechnet wurden – nur zur Anzeige (Badge in der Übersicht).
func (t Trade) UnsettledExitsPnL() F32 {
	var sum F32
	for _, e := range t.Exits {
		if !e.Settled && e.Price > 0 {
			sum += t.ExitPnL(e) * t.exitFx(e)
		}
	}
	return sum
}

// exitFx liefert den Wechselkurs eines Exits in Kontowährung: bevorzugt den
// exit-eigenen Kurs, sonst den Trade-Kurs (AcctFx).
func (t Trade) exitFx(e Exit) F32 {
	if e.FxRate > 0 {
		return F32(e.FxRate)
	}
	return t.AcctFx()
}

// NeedsReconcile meldet, ob es einen realen Exit (mit Preis) gibt, der noch
// nicht mit dem Kontostand verrechnet wurde.
func (t Trade) NeedsReconcile() bool {
	for _, e := range t.Exits {
		if e.Price > 0 && !e.Settled {
			return true
		}
	}
	return false
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

// RiskAmount ist das tatsächliche Risiko in Trade-Währung: Stückzahl × Kursabstand,
// richtungsabhängig. Negativ, wenn der Stop bereits im Gewinn liegt (kein Risiko).
func (t Trade) RiskAmount() F32 {
	qty := t.EffectiveQty()
	if qty <= 0 || t.EntryPrice <= 0 || t.StopLoss <= 0 {
		return 0
	}
	return qty * t.signedDiff()
}

// AcctFx ist der Umrechnungsfaktor von der Trade-Währung in die Kontowährung.
// Ohne gesetzten Kurs 1 (Trade läuft in Kontowährung).
func (t Trade) AcctFx() F32 {
	if t.FxRate > 0 {
		return F32(t.FxRate)
	}
	return 1
}

// RiskAmountAcct ist das Risiko in Kontowährung (RiskAmount × Wechselkurs).
func (t Trade) RiskAmountAcct() F32 {
	return t.RiskAmount() * t.AcctFx()
}

// MarginAcct ist die gebundene Margin in Kontowährung.
func (t Trade) MarginAcct() F32 {
	return t.Margin * t.AcctFx()
}

// PositionValueAcct ist der Positionswert (Notional) in Kontowährung.
func (t Trade) PositionValueAcct() F32 {
	return t.PositionValue * t.AcctFx()
}

// RiskPercent ist das Risiko als Anteil der Kontogröße in Prozent.
func (t Trade) RiskPercent(accountSize F32) F32 {
	if accountSize <= 0 {
		return 0
	}
	return t.RiskAmountAcct() / accountSize * 100
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
