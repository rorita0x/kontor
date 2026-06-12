package src

import "testing"

func approx(a, b F32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.01
}

func TestRealizedPnL(t *testing.T) {
	// Long: 10 Stück, Entry 100. Voll bei 110 raus → +100.
	long := Trade{
		Traded: TRADED_LONG, EntryPrice: 100, Quantity: 10,
		Exits: []Exit{{Price: 110, Quantity: 100}},
	}
	if got := long.RealizedPnL(); !approx(got, 100) {
		t.Errorf("long voll: got %v, want 100", got)
	}

	// Long mit Teil-Exits: 50% @110 (+50), 50% @90 (−50) → 0.
	longSplit := Trade{
		Traded: TRADED_LONG, EntryPrice: 100, Quantity: 10,
		Exits: []Exit{{Price: 110, Quantity: 50}, {Price: 90, Quantity: 50}},
	}
	if got := longSplit.RealizedPnL(); !approx(got, 0) {
		t.Errorf("long split: got %v, want 0", got)
	}

	// Short: 10 Stück, Entry 100, Exit 90 → +100 (Gewinn bei fallendem Kurs).
	short := Trade{
		Traded: TRADED_SHORT, EntryPrice: 100, Quantity: 10,
		Exits: []Exit{{Price: 90, Quantity: 100}},
	}
	if got := short.RealizedPnL(); !approx(got, 100) {
		t.Errorf("short voll: got %v, want 100", got)
	}

	// Positionswert statt Stückzahl: 1000 @ Entry 100 = 10 Stück, Exit 110 → +100.
	byValue := Trade{
		Traded: TRADED_LONG, EntryPrice: 100, PositionValue: 1000,
		Exits: []Exit{{Price: 110, Quantity: 100}},
	}
	if got := byValue.RealizedPnL(); !approx(got, 100) {
		t.Errorf("by value: got %v, want 100", got)
	}
}

func TestUnsettledAndNeedsReconcile(t *testing.T) {
	tr := Trade{
		Traded: TRADED_LONG, EntryPrice: 100, Quantity: 10,
		Exits: []Exit{{Price: 110, Quantity: 100}}, // realized +100
	}
	if got := tr.UnsettledPnL(); !approx(got, 100) {
		t.Errorf("unsettled (nichts verrechnet): got %v, want 100", got)
	}
	if !tr.NeedsReconcile() {
		t.Error("NeedsReconcile sollte true sein, solange nichts verrechnet ist")
	}

	tr.Settled = 100 // vollständig verrechnet
	if got := tr.UnsettledPnL(); !approx(got, 0) {
		t.Errorf("unsettled (voll verrechnet): got %v, want 0", got)
	}
	if tr.NeedsReconcile() {
		t.Error("NeedsReconcile sollte false sein, wenn alles verrechnet ist")
	}

	// Ohne Exits: kein realisiertes Ergebnis, kein Stups.
	open := Trade{Traded: TRADED_LONG, EntryPrice: 100, Quantity: 10}
	if open.NeedsReconcile() {
		t.Error("offener Trade ohne Exits sollte nicht zum Verrechnen stupsen")
	}
}
