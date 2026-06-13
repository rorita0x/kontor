package src

import "testing"

func TestEffectiveQty(t *testing.T) {
	// Direkt eingegebene Stückzahl hat Vorrang.
	tr := Trade{Quantity: 7, PositionValue: 1000, EntryPrice: 100}
	if got := tr.EffectiveQty(); !approx(got, 7) {
		t.Errorf("Quantity-Vorrang: got %v, want 7", got)
	}

	// Ohne Stückzahl aus Positionswert / Entry abgeleitet.
	byValue := Trade{PositionValue: 1000, EntryPrice: 50}
	if got := byValue.EffectiveQty(); !approx(got, 20) {
		t.Errorf("aus Positionswert: got %v, want 20", got)
	}

	// Weder Stückzahl noch (Positionswert + Entry) → 0.
	if got := (Trade{PositionValue: 1000}).EffectiveQty(); !approx(got, 0) {
		t.Errorf("ohne Entry: got %v, want 0", got)
	}
	if got := (Trade{}).EffectiveQty(); !approx(got, 0) {
		t.Errorf("leer: got %v, want 0", got)
	}
}

func TestRiskFromSL(t *testing.T) {
	// Long: Entry 100, Stop 95 → 5 % Abstand in Verlustrichtung.
	long := Trade{Traded: TRADED_LONG, EntryPrice: 100, StopLoss: 95}
	if got := long.RiskFromSL(); !approx(got, 5) {
		t.Errorf("long: got %v, want 5", got)
	}

	// Short: Verlustrichtung ist nach oben, Stop 105 → 5 %.
	short := Trade{Traded: TRADED_SHORT, EntryPrice: 100, StopLoss: 105}
	if got := short.RiskFromSL(); !approx(got, 5) {
		t.Errorf("short: got %v, want 5", got)
	}

	// Long mit Stop im Gewinn (110 > Entry 100) → negativer Abstand.
	secured := Trade{Traded: TRADED_LONG, EntryPrice: 100, StopLoss: 110}
	if got := secured.RiskFromSL(); !approx(got, -10) {
		t.Errorf("Stop im Gewinn: got %v, want -10", got)
	}

	// Ohne Entry oder ohne Stop → 0.
	if got := (Trade{EntryPrice: 100}).RiskFromSL(); !approx(got, 0) {
		t.Errorf("ohne Stop: got %v, want 0", got)
	}
	if got := (Trade{StopLoss: 95}).RiskFromSL(); !approx(got, 0) {
		t.Errorf("ohne Entry: got %v, want 0", got)
	}
}

func TestRiskAmount(t *testing.T) {
	// Long: 10 Stück, Entry 100, Stop 95 → 10 * 5 = 50.
	long := Trade{Traded: TRADED_LONG, EntryPrice: 100, StopLoss: 95, Quantity: 10}
	if got := long.RiskAmount(); !approx(got, 50) {
		t.Errorf("long: got %v, want 50", got)
	}

	// Short: 10 Stück, Entry 100, Stop 105 → 10 * 5 = 50.
	short := Trade{Traded: TRADED_SHORT, EntryPrice: 100, StopLoss: 105, Quantity: 10}
	if got := short.RiskAmount(); !approx(got, 50) {
		t.Errorf("short: got %v, want 50", got)
	}

	// Über Positionswert abgeleitete Stückzahl: 1000 / 100 = 10 Stück.
	byValue := Trade{Traded: TRADED_LONG, EntryPrice: 100, StopLoss: 95, PositionValue: 1000}
	if got := byValue.RiskAmount(); !approx(got, 50) {
		t.Errorf("aus Positionswert: got %v, want 50", got)
	}

	// Stop im Gewinn → negatives "Risiko".
	secured := Trade{Traded: TRADED_LONG, EntryPrice: 100, StopLoss: 110, Quantity: 10}
	if got := secured.RiskAmount(); !approx(got, -100) {
		t.Errorf("Stop im Gewinn: got %v, want -100", got)
	}

	// Ohne brauchbare Eingaben → 0.
	if got := (Trade{Traded: TRADED_LONG, EntryPrice: 100, Quantity: 10}).RiskAmount(); !approx(got, 0) {
		t.Errorf("ohne Stop: got %v, want 0", got)
	}
}

func TestRiskPercent(t *testing.T) {
	// Risiko 50 bei Konto 10000 → 0,5 %.
	tr := Trade{Traded: TRADED_LONG, EntryPrice: 100, StopLoss: 95, Quantity: 10}
	if got := tr.RiskPercent(10000); !approx(got, 0.5) {
		t.Errorf("got %v, want 0.5", got)
	}

	// Kontogröße 0 oder negativ → 0 (Division vermeiden).
	if got := tr.RiskPercent(0); !approx(got, 0) {
		t.Errorf("Konto 0: got %v, want 0", got)
	}
	if got := tr.RiskPercent(-100); !approx(got, 0) {
		t.Errorf("Konto negativ: got %v, want 0", got)
	}
}

func TestTradeResultIsCorrect(t *testing.T) {
	if !TradeResult(RESULT_WIN).IsCorrect() {
		t.Error("Gewinn sollte als korrekt gelten")
	}
	for _, r := range []TradeResult{RESULT_LOSS, RESULT_BREAKEVEN, RESULT_NOTFINISHED, RESULT_SKIP} {
		if r.IsCorrect() {
			t.Errorf("%q sollte nicht als korrekt gelten", r)
		}
	}
}

func TestTradeResultColor(t *testing.T) {
	cases := map[TradeResult]string{
		RESULT_WIN:         "success",
		RESULT_LOSS:        "danger",
		RESULT_BREAKEVEN:   "warning",
		RESULT_NOTFINISHED: "info",
		RESULT_SKIP:        "secondary", // default
	}
	for r, want := range cases {
		if got := r.Color(); got != want {
			t.Errorf("Color(%q): got %q, want %q", r, got, want)
		}
	}
}

func TestTradeResultDisplay(t *testing.T) {
	cases := map[TradeResult]string{
		RESULT_WIN:         "Gewinn",
		RESULT_LOSS:        "Verlust",
		RESULT_BREAKEVEN:   "Break-Even",
		RESULT_NOTFINISHED: "Offen",
		RESULT_SKIP:        "Skip",
	}
	for r, want := range cases {
		if got := r.Display(); got != want {
			t.Errorf("Display(%q): got %q, want %q", r, got, want)
		}
	}
	// Unbekanntes Ergebnis wird unverändert als String zurückgegeben.
	if got := TradeResult("foo").Display(); got != "foo" {
		t.Errorf("unbekannt: got %q, want %q", got, "foo")
	}
}

func TestF32TextRoundtrip(t *testing.T) {
	var f F32
	if err := f.UnmarshalText([]byte("12.34")); err != nil {
		t.Fatalf("UnmarshalText: unerwarteter Fehler: %v", err)
	}
	if !approx(f, 12.34) {
		t.Errorf("UnmarshalText: got %v, want 12.34", f)
	}

	// MarshalText rundet auf zwei Nachkommastellen.
	b, err := F32(12.345).MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: unerwarteter Fehler: %v", err)
	}
	if string(b) != "12.35" && string(b) != "12.34" {
		t.Errorf("MarshalText: got %q, want ~12.34/12.35", string(b))
	}

	// String entspricht der Textdarstellung mit zwei Nachkommastellen.
	if got := F32(5).String(); got != "5.00" {
		t.Errorf("String: got %q, want %q", got, "5.00")
	}

	// Ungültiger Text liefert einen Fehler.
	if err := f.UnmarshalText([]byte("nicht-zahl")); err == nil {
		t.Error("UnmarshalText sollte bei ungültigem Text einen Fehler liefern")
	}
}

func TestComputeMarginBuffer(t *testing.T) {
	// Long, Entry 100, Stop 95 (5 % Abstand), Stückzahl 100 → Risiko 500.
	// Margin 1000 (z. B. Hebel 10 auf Notional 10000); am Stop 1000 × 0,95 = 950.
	tr := Trade{
		Symbol: "GOLD", Traded: TRADED_LONG, Result: RESULT_NOTFINISHED,
		EntryPrice: 100, StopLoss: 95, Quantity: 100, Margin: 1000,
	}

	// Ziel 25 % → Faktor 50/75 = 0,6667. Beitrag = 500 + 0,6667·950 = 1133,33.
	b25 := computeMarginBuffer([]Trade{tr}, 25)
	if b25.OpenCount != 1 {
		t.Fatalf("OpenCount: got %d, want 1", b25.OpenCount)
	}
	if !approx(b25.TotalRisk, 500) {
		t.Errorf("TotalRisk: got %v, want 500", b25.TotalRisk)
	}
	if !approx(b25.TotalMarginStop, 950) {
		t.Errorf("TotalMarginStop: got %v, want 950", b25.TotalMarginStop)
	}
	if !approx(b25.RequiredDeposit, 1133.33) {
		t.Errorf("RequiredDeposit 25%%: got %v, want 1133.33", b25.RequiredDeposit)
	}
	if b25.FlooredByMargin {
		t.Error("25%% sollte nicht von der Entry-Margin begrenzt sein")
	}

	// Ziel 45 % → Faktor 50/55 = 0,9091. Beitrag = 500 + 0,9091·950 = 1363,64.
	b45 := computeMarginBuffer([]Trade{tr}, 45)
	if !approx(b45.RequiredDeposit, 1363.64) {
		t.Errorf("RequiredDeposit 45%%: got %v, want 1363.64", b45.RequiredDeposit)
	}

	// Entry-Margin als Untergrenze: kleines Risiko, große Margin.
	// Stop 99 (1 %), Stückzahl 10 → Risiko 10; Margin 1000, am Stop 990.
	// Ziel 25 %: 10 + 0,6667·990 = 670 < 1000 → Einzahlung = 1000, gefloored.
	tiny := Trade{
		Symbol: "TINY", Traded: TRADED_LONG, Result: RESULT_NOTFINISHED,
		EntryPrice: 100, StopLoss: 99, Quantity: 10, Margin: 1000,
	}
	bFloor := computeMarginBuffer([]Trade{tiny}, 25)
	if !bFloor.FlooredByMargin {
		t.Error("erwartete Begrenzung durch Entry-Margin")
	}
	if !approx(bFloor.RequiredDeposit, 1000) {
		t.Errorf("gefloorte Einzahlung: got %v, want 1000", bFloor.RequiredDeposit)
	}

	// Abgeschlossene Trades zählen nicht mit.
	closed := Trade{Symbol: "X", Result: RESULT_WIN, EntryPrice: 100, StopLoss: 95, Quantity: 100, Margin: 1000}
	if got := computeMarginBuffer([]Trade{closed}, 25); got.OpenCount != 0 {
		t.Errorf("abgeschlossener Trade: OpenCount got %d, want 0", got.OpenCount)
	}
}
