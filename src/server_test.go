package src

import "testing"

// openTrade ist ein Helfer für einen offenen Long-Trade mit definiertem Risiko.
func openTrade(symbol string, entry, stop, qty F32) Trade {
	return Trade{
		Symbol: symbol, Traded: TRADED_LONG, Result: RESULT_NOTFINISHED,
		EntryPrice: entry, StopLoss: stop, Quantity: qty,
	}
}

func TestComputeStats(t *testing.T) {
	trades := []Trade{
		{Result: RESULT_WIN},
		{Result: RESULT_WIN},
		{Result: RESULT_LOSS},
		{Result: RESULT_BREAKEVEN},
		{Result: RESULT_SKIP},
		openTrade("AAA", 100, 95, 10), // Risiko 50
	}
	s := computeStats(trades, 10000)

	if s.Total != 6 {
		t.Errorf("Total: got %d, want 6", s.Total)
	}
	if s.Wins != 2 || s.Losses != 1 || s.BreakEven != 1 || s.Skips != 1 || s.Open != 1 {
		t.Errorf("Zählung falsch: %+v", s)
	}
	// WinRate = Wins / (Wins+Losses) = 2/3 ≈ 66,67 %.
	if !approx(s.WinRate, 66.6667) {
		t.Errorf("WinRate: got %v, want ~66.67", s.WinRate)
	}
	// Offenes Risiko nur aus dem offenen Trade: 50 absolut, 0,5 % vom Konto.
	if !approx(s.OpenRisk, 50) {
		t.Errorf("OpenRisk: got %v, want 50", s.OpenRisk)
	}
	if !approx(s.OpenRiskPct, 0.5) {
		t.Errorf("OpenRiskPct: got %v, want 0.5", s.OpenRiskPct)
	}
}

func TestComputeStatsNoDecided(t *testing.T) {
	// Ohne entschiedene Trades bleibt die WinRate 0 (keine Division durch 0).
	s := computeStats([]Trade{{Result: RESULT_SKIP}, {Result: RESULT_NOTFINISHED}}, 10000)
	if !approx(s.WinRate, 0) {
		t.Errorf("WinRate: got %v, want 0", s.WinRate)
	}
}

func TestComputeClassRisk(t *testing.T) {
	trades := []Trade{
		openTrade("AAA", 100, 95, 10),  // Risiko 50, Klasse Krypto
		openTrade("BBB", 100, 90, 10),  // Risiko 100, Klasse Krypto
		{Symbol: "AAA", Result: RESULT_WIN, EntryPrice: 100, StopLoss: 95, Quantity: 10}, // zählt nicht (geschlossen)
	}
	assets := []Asset{
		{Symbol: "AAA", AssetClass: "Krypto"},
		{Symbol: "BBB", AssetClass: "Krypto"},
	}
	classes := []AssetClass{{Title: "Krypto", RiskLimit: 5}}

	result := computeClassRisk(trades, assets, classes, 10000)
	if len(result) != 1 {
		t.Fatalf("erwarte 1 Klasse, got %d (%+v)", len(result), result)
	}
	kr := result[0]
	if kr.Class != "Krypto" {
		t.Errorf("Class: got %q", kr.Class)
	}
	if kr.TradeCount != 2 {
		t.Errorf("TradeCount: got %d, want 2", kr.TradeCount)
	}
	// Risiko gesamt 150 absolut, 1,5 % vom Konto.
	if !approx(kr.TotalRiskAmount, 150) {
		t.Errorf("TotalRiskAmount: got %v, want 150", kr.TotalRiskAmount)
	}
	if !approx(kr.TotalRisk, 1.5) {
		t.Errorf("TotalRisk: got %v, want 1.5", kr.TotalRisk)
	}
	// Limit 5 % → 500 absolut, davon noch 350 / 3,5 % frei.
	if !kr.HasLimit || !approx(kr.LimitAmount, 500) {
		t.Errorf("Limit falsch: %+v", kr)
	}
	if !approx(kr.FreePct, 3.5) || !approx(kr.FreeAmount, 350) {
		t.Errorf("Frei falsch: FreePct=%v FreeAmount=%v", kr.FreePct, kr.FreeAmount)
	}
}

func TestComputeClassRiskLimitWithoutTrades(t *testing.T) {
	// Eine Klasse mit Limit erscheint auch ohne offene Trades (voller Spielraum).
	classes := []AssetClass{{Title: "Aktien", RiskLimit: 4}}
	result := computeClassRisk(nil, nil, classes, 10000)
	if len(result) != 1 {
		t.Fatalf("erwarte 1 Klasse, got %d", len(result))
	}
	if !approx(result[0].FreePct, 4) || !approx(result[0].FreeAmount, 400) {
		t.Errorf("voller Spielraum erwartet: %+v", result[0])
	}
}

func TestComputeClassRiskUnclassified(t *testing.T) {
	// Trades ohne passendes Asset landen in "Unclassified", ohne Limit.
	trades := []Trade{openTrade("ZZZ", 100, 95, 10)}
	result := computeClassRisk(trades, nil, nil, 10000)
	if len(result) != 1 || result[0].Class != "Unclassified" {
		t.Fatalf("erwarte Unclassified, got %+v", result)
	}
	if result[0].HasLimit {
		t.Error("Unclassified ohne Limit erwartet")
	}
}

func TestComputeSymbolRisk(t *testing.T) {
	trades := []Trade{
		openTrade("AAA", 100, 95, 10), // Risiko 50 / 0,5 %
		openTrade("AAA", 100, 95, 10), // noch mal 50 → zusammen 100 / 1 %
	}
	assets := []Asset{{Symbol: "AAA", AssetClass: "Krypto"}}
	// Asset-Limit großzügig (10 %), Sektor-Limit eng (1,5 %) → Sektor bindet.
	classes := []AssetClass{{Title: "Krypto", RiskLimit: 1.5}}
	classRisk := computeClassRisk(trades, assets, classes, 10000)

	result := computeSymbolRisk(trades, assets, 10, 10000, classRisk)
	if len(result) != 1 {
		t.Fatalf("erwarte 1 Symbol, got %d", len(result))
	}
	sr := result[0]
	if sr.TradeCount != 2 || !approx(sr.TotalRiskAmount, 100) {
		t.Errorf("Aggregation falsch: %+v", sr)
	}
	// Asset-frei wäre 10 % − 1 % = 9 %; Sektor-frei 1,5 % − 1 % = 0,5 % ist enger.
	if !sr.SectorBinds {
		t.Error("Sektor sollte die bindende Grenze sein")
	}
	if !approx(sr.FreePct, 0.5) {
		t.Errorf("FreePct: got %v, want 0.5 (Sektor-frei)", sr.FreePct)
	}
}

func TestComputeSymbolRiskAssetBinds(t *testing.T) {
	trades := []Trade{openTrade("AAA", 100, 95, 10)} // 50 / 0,5 %
	assets := []Asset{{Symbol: "AAA", AssetClass: "Krypto"}}
	// Asset-Limit eng (1 %), Sektor-Limit großzügig (10 %) → Asset bindet.
	classes := []AssetClass{{Title: "Krypto", RiskLimit: 10}}
	classRisk := computeClassRisk(trades, assets, classes, 10000)

	result := computeSymbolRisk(trades, assets, 1, 10000, classRisk)
	if len(result) != 1 {
		t.Fatalf("erwarte 1 Symbol, got %d", len(result))
	}
	sr := result[0]
	if sr.SectorBinds {
		t.Error("Asset sollte binden, nicht der Sektor")
	}
	// Asset-frei = 1 % − 0,5 % = 0,5 %.
	if !approx(sr.FreePct, 0.5) {
		t.Errorf("FreePct: got %v, want 0.5 (Asset-frei)", sr.FreePct)
	}
}

func TestFormRiskDataExcludesPk(t *testing.T) {
	trades := []Trade{
		{Pk: 1, Symbol: "AAA", Traded: TRADED_LONG, Result: RESULT_NOTFINISHED, EntryPrice: 100, StopLoss: 95, Quantity: 10},
		{Pk: 2, Symbol: "AAA", Traded: TRADED_LONG, Result: RESULT_NOTFINISHED, EntryPrice: 100, StopLoss: 95, Quantity: 10},
		{Pk: 3, Symbol: "AAA", Traded: TRADED_LONG, Result: RESULT_WIN, EntryPrice: 100, StopLoss: 95, Quantity: 10}, // geschlossen, zählt nie
	}
	assets := []Asset{{Symbol: "AAA", AssetClass: "Krypto"}}

	// Ohne Ausschluss: beide offenen Trades → 100 absolut.
	symbolRisk, classRisk, symbolClass, classLimits := formRiskData(trades, assets, []AssetClass{{Title: "Krypto", RiskLimit: 5}}, 10000, 0)
	if !approx(F32(symbolRisk["AAA"]["amount"]), 100) {
		t.Errorf("ohne Ausschluss: got %v, want 100", symbolRisk["AAA"]["amount"])
	}
	if !approx(F32(classRisk["Krypto"]["amount"]), 100) {
		t.Errorf("Klasse ohne Ausschluss: got %v, want 100", classRisk["Krypto"]["amount"])
	}
	if symbolClass["AAA"] != "Krypto" {
		t.Errorf("symbolClass: got %q, want Krypto", symbolClass["AAA"])
	}
	if !approx(F32(classLimits["Krypto"]), 5) {
		t.Errorf("classLimits: got %v, want 5", classLimits["Krypto"])
	}

	// Mit Ausschluss von Pk 1: nur noch ein offener Trade → 50 absolut.
	symbolRisk, _, _, _ = formRiskData(trades, assets, nil, 10000, 1)
	if !approx(F32(symbolRisk["AAA"]["amount"]), 50) {
		t.Errorf("mit Ausschluss Pk1: got %v, want 50", symbolRisk["AAA"]["amount"])
	}
}

func TestFlashMessage(t *testing.T) {
	cases := []struct {
		code    string
		wantMsg bool
		typ     string
	}{
		{"trade-saved", true, "success"},
		{"trade-updated", true, "success"},
		{"trade-deleted", true, "success"},
		{"settings-saved", true, "success"},
		{"reconcile-needed", true, "warning"},
		{"reconciled", true, "success"},
		{"unbekannt", false, ""},
		{"", false, ""},
	}
	for _, tc := range cases {
		msg, typ := flashMessage(tc.code)
		if tc.wantMsg && msg == "" {
			t.Errorf("%q: erwarte nicht-leere Meldung", tc.code)
		}
		if !tc.wantMsg && msg != "" {
			t.Errorf("%q: erwarte leere Meldung, got %q", tc.code, msg)
		}
		if typ != tc.typ {
			t.Errorf("%q: typ got %q, want %q", tc.code, typ, tc.typ)
		}
	}
}
