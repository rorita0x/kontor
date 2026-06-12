package src

import "testing"

func approx(a, b F32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.01
}

func TestExitPnL(t *testing.T) {
	// Long: 10 Stück, Entry 100. Voll bei 110 raus → +100.
	long := Trade{Traded: TRADED_LONG, EntryPrice: 100, Quantity: 10}
	if got := long.ExitPnL(Exit{Price: 110, Quantity: 100}); !approx(got, 100) {
		t.Errorf("long voll: got %v, want 100", got)
	}
	// Teil-Exit: 50% @110 → +50.
	if got := long.ExitPnL(Exit{Price: 110, Quantity: 50}); !approx(got, 50) {
		t.Errorf("long 50%%: got %v, want 50", got)
	}

	// Short: 10 Stück, Entry 100, Exit 90 → +100 (Gewinn bei fallendem Kurs).
	short := Trade{Traded: TRADED_SHORT, EntryPrice: 100, Quantity: 10}
	if got := short.ExitPnL(Exit{Price: 90, Quantity: 100}); !approx(got, 100) {
		t.Errorf("short voll: got %v, want 100", got)
	}

	// Positionswert statt Stückzahl: 1000 @ Entry 100 = 10 Stück, Exit 110 → +100.
	byValue := Trade{Traded: TRADED_LONG, EntryPrice: 100, PositionValue: 1000}
	if got := byValue.ExitPnL(Exit{Price: 110, Quantity: 100}); !approx(got, 100) {
		t.Errorf("by value: got %v, want 100", got)
	}

	// Ohne Preis: kein Ergebnis.
	if got := long.ExitPnL(Exit{Quantity: 100}); got != 0 {
		t.Errorf("ohne Preis: got %v, want 0", got)
	}
}

func TestSettledTotalAndUnsettled(t *testing.T) {
	tr := Trade{
		Traded: TRADED_LONG, EntryPrice: 100, Quantity: 10,
		Exits: []Exit{
			{Price: 110, Quantity: 50, Settled: true, SettledAmount: 48}, // verrechnet (mit Abzug)
			{Price: 120, Quantity: 50},                                   // offen, berechnet +100
		},
	}
	if got := tr.SettledTotal(); !approx(got, 48) {
		t.Errorf("SettledTotal: got %v, want 48", got)
	}
	if got := tr.UnsettledExitsPnL(); !approx(got, 100) {
		t.Errorf("UnsettledExitsPnL: got %v, want 100", got)
	}
	if !tr.NeedsReconcile() {
		t.Error("NeedsReconcile sollte true sein, solange ein Exit offen ist")
	}

	// Beide Exits verrechnet → kein Stups mehr.
	tr.Exits[1].Settled = true
	if tr.NeedsReconcile() {
		t.Error("NeedsReconcile sollte false sein, wenn alle Exits verrechnet sind")
	}
}

func TestNeedsReconcileNoExits(t *testing.T) {
	// Offener Trade ohne Exits: kein Stups.
	open := Trade{Traded: TRADED_LONG, EntryPrice: 100, Quantity: 10}
	if open.NeedsReconcile() {
		t.Error("Trade ohne Exits sollte nicht zum Verrechnen stupsen")
	}
	// Exit-Zeile ohne Preis (unvollständig): kein Stups.
	draft := Trade{Traded: TRADED_LONG, EntryPrice: 100, Quantity: 10, Exits: []Exit{{Quantity: 50}}}
	if draft.NeedsReconcile() {
		t.Error("Exit ohne Preis sollte nicht zum Verrechnen stupsen")
	}
}
