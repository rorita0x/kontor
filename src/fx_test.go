package src

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// Risiko/Margin in Kontowährung = Trade-Werte × Wechselkurs.
func TestFxConversionMethods(t *testing.T) {
	// Long BTC: Entry 100, Stop 90 → 10 je Stück, 10 Stück = 100 USD Risiko.
	tr := Trade{
		Traded: TRADED_LONG, Result: RESULT_NOTFINISHED,
		EntryPrice: 100, StopLoss: 90, Quantity: 10,
		PositionValue: 1000, Margin: 200, FxRate: 0.8638,
	}

	if !approx(tr.RiskAmount(), 100) {
		t.Errorf("RiskAmount (Trade-Währung): got %v, want 100", tr.RiskAmount())
	}
	if !approx(tr.RiskAmountAcct(), 86.38) {
		t.Errorf("RiskAmountAcct (EUR): got %v, want 86.38", tr.RiskAmountAcct())
	}
	if !approx(tr.MarginAcct(), 172.76) {
		t.Errorf("MarginAcct (EUR): got %v, want 172.76", tr.MarginAcct())
	}
	if !approx(tr.PositionValueAcct(), 863.8) {
		t.Errorf("PositionValueAcct (EUR): got %v, want 863.8", tr.PositionValueAcct())
	}
	// 86.38 € Risiko auf 10000 € Konto = 0.8638 %.
	if got := tr.RiskPercent(10000); !approx(got, 0.8638) {
		t.Errorf("RiskPercent: got %v, want 0.8638", got)
	}

	// Ohne Kurs (FxRate 0) bleibt alles in Kontowährung (Faktor 1).
	tr.FxRate = 0
	if !approx(tr.RiskAmountAcct(), 100) || !approx(tr.MarginAcct(), 200) {
		t.Errorf("ohne FxRate: RiskAmountAcct=%v MarginAcct=%v, want 100/200", tr.RiskAmountAcct(), tr.MarginAcct())
	}
}

// Exit-eigener Kurs hat Vorrang vor dem Trade-Kurs; sonst Fallback auf Trade-Kurs.
func TestExitFxFallback(t *testing.T) {
	tr := Trade{FxRate: 0.90}
	if got := tr.exitFx(Exit{FxRate: 0.85}); !approx(got, 0.85) {
		t.Errorf("exit-eigener Kurs: got %v, want 0.85", got)
	}
	if got := tr.exitFx(Exit{}); !approx(got, 0.90) {
		t.Errorf("Fallback Trade-Kurs: got %v, want 0.90", got)
	}
}

// /insert bindet fxRate und speichert es; leeres Feld bleibt 0 (rückwärtskompatibel).
func TestInsertBindsFxRate(t *testing.T) {
	db := setupRoutes(t, 10000)
	r := gin.New()
	CreateRoutes(db, r)

	// Mit Trade-Kurs und einem Exit, der einen abweichenden Kurs trägt.
	postInsert(t, r, map[string]string{
		"symbol": "AAPL", "traded": "long", "result": "not finished",
		"entryPrice": "100", "stopLoss": "90", "quantity": "10",
		"margin": "200", "fxRate": "0.8638",
		"exitsJSON": `[{"date":"2026-06-12","price":"110","qty":"50","settled":false,"settledAmount":"","fxRate":"0.85"}]`,
	})

	var trades []Trade
	db.All(&trades)
	if len(trades) != 1 {
		t.Fatalf("Trades: got %d, want 1", len(trades))
	}
	got := trades[0]
	if got.FxRate < 0.8637 || got.FxRate > 0.8639 {
		t.Errorf("Trade.FxRate: got %v, want ~0.8638", got.FxRate)
	}
	if len(got.Exits) != 1 || got.Exits[0].FxRate < 0.8499 || got.Exits[0].FxRate > 0.8501 {
		t.Errorf("Exit.FxRate: got %+v, want ~0.85", got.Exits)
	}
	// Risiko in EUR muss den Trade-Kurs anwenden (100 USD × 0.8638 = 86.38 €).
	if !approx(got.RiskAmountAcct(), 86.38) {
		t.Errorf("RiskAmountAcct: got %v, want 86.38", got.RiskAmountAcct())
	}
}

// Ohne fxRate-Feld (alte Formulare / EUR-Trades) bleibt FxRate 0 und Insert klappt.
func TestInsertWithoutFxRate(t *testing.T) {
	db := setupRoutes(t, 10000)
	r := gin.New()
	CreateRoutes(db, r)

	postInsert(t, r, map[string]string{
		"symbol": "BTC", "traded": "long", "result": "not finished",
		"entryPrice": "100", "stopLoss": "90", "quantity": "10",
	})

	var trades []Trade
	db.All(&trades)
	if len(trades) != 1 {
		t.Fatalf("Trades: got %d, want 1", len(trades))
	}
	if trades[0].FxRate != 0 {
		t.Errorf("FxRate ohne Feld: got %v, want 0", trades[0].FxRate)
	}
	if !approx(trades[0].RiskAmountAcct(), 100) {
		t.Errorf("RiskAmountAcct ohne Kurs: got %v, want 100", trades[0].RiskAmountAcct())
	}
}
