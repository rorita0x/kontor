package src

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/asdine/storm/v3"
	"github.com/gin-gonic/gin"
)

// multipartBody baut einen multipart/form-data-Body aus einfachen Feldern
// (so wie das Eintrags-Formular ihn sendet – /insert liest c.MultipartForm()).
func multipartBody(fields map[string]string) (*bytes.Buffer, string) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	return buf, w.FormDataContentType()
}

func postInsert(t *testing.T, r *gin.Engine, fields map[string]string) {
	t.Helper()
	body, ct := multipartBody(fields)
	req := httptest.NewRequest(http.MethodPost, "/insert", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusFound { // /insert leitet immer per 302 weiter
		t.Fatalf("/insert: status got %d, want 302 (body: %s)", w.Code, w.Body.String())
	}
}

func setupRoutes(t *testing.T, startBalance F32) *storm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := storm.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("storm.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Save(&Settings{Pk: 1, AccountSize: startBalance}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	return db
}

func TestInsertBooksReconciledExit(t *testing.T) {
	db := setupRoutes(t, 5000)
	r := gin.New()
	CreateRoutes(db, r)

	// Neuer Trade, ein Exit als verrechnet markiert (250 €).
	postInsert(t, r, map[string]string{
		"symbol": "BTC", "traded": "long", "result": "win",
		"entryPrice": "100", "quantity": "10",
		"exitsJSON": `[{"date":"2026-06-12","price":"110","qty":"100","settled":true,"settledAmount":"250"}]`,
	})

	var s Settings
	if err := db.One("Pk", 1, &s); err != nil {
		t.Fatal(err)
	}
	if !approx(s.AccountSize, 5250) {
		t.Errorf("Kontostand nach Verrechnen: got %v, want 5250", s.AccountSize)
	}

	var trades []Trade
	db.All(&trades)
	if len(trades) != 1 {
		t.Fatalf("Trades: got %d, want 1", len(trades))
	}
	e := trades[0].Exits[0]
	if !e.Settled || !approx(e.SettledAmount, 250) {
		t.Errorf("Exit nicht korrekt gespeichert: %+v", e)
	}
}

func TestInsertNoDoubleBookingAndUnticking(t *testing.T) {
	db := setupRoutes(t, 5000)
	r := gin.New()
	CreateRoutes(db, r)

	settled := `[{"date":"2026-06-12","price":"110","qty":"100","settled":true,"settledAmount":"250"}]`

	// 1) Anlegen mit verrechnetem Exit → 5250.
	postInsert(t, r, map[string]string{
		"symbol": "BTC", "traded": "long", "result": "win",
		"entryPrice": "100", "quantity": "10", "exitsJSON": settled,
	})

	// 2) Erneut speichern, unverändert (id=1) → keine Doppelbuchung, bleibt 5250.
	postInsert(t, r, map[string]string{
		"id": "1", "symbol": "BTC", "traded": "long", "result": "win",
		"entryPrice": "100", "quantity": "10", "exitsJSON": settled,
	})
	var s Settings
	db.One("Pk", 1, &s)
	if !approx(s.AccountSize, 5250) {
		t.Errorf("nach Re-Save: got %v, want 5250 (Doppelbuchung?)", s.AccountSize)
	}

	// 3) Häkchen wieder entfernen → 250 zurückgebucht, 5000.
	postInsert(t, r, map[string]string{
		"id": "1", "symbol": "BTC", "traded": "long", "result": "win",
		"entryPrice": "100", "quantity": "10",
		"exitsJSON": `[{"date":"2026-06-12","price":"110","qty":"100","settled":false,"settledAmount":"250"}]`,
	})
	db.One("Pk", 1, &s)
	if !approx(s.AccountSize, 5000) {
		t.Errorf("nach Abwählen: got %v, want 5000", s.AccountSize)
	}
}
