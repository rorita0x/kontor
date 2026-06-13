package src

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/asdine/storm/v3"
	"github.com/gin-gonic/gin"
)

// fetchFxRate holt den aktuellen Wechselkurs für ein Yahoo-Finance-Währungspaar
// (z. B. "USDEUR=X" → EUR je USD) aus meta.regularMarketPrice.
func fetchFxRate(pair string) (float64, error) {
	url := "https://query1.finance.yahoo.com/v8/finance/chart/" + pair + "?interval=1d&range=1d"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	// Ohne User-Agent antwortet Yahoo teils mit 429/403.
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("yahoo HTTP %d", resp.StatusCode)
	}
	var data struct {
		Chart struct {
			Result []struct {
				Meta struct {
					RegularMarketPrice float64 `json:"regularMarketPrice"`
				} `json:"meta"`
			} `json:"result"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}
	if len(data.Chart.Result) == 0 || data.Chart.Result[0].Meta.RegularMarketPrice == 0 {
		return 0, fmt.Errorf("kein Kurs in der Antwort")
	}
	return data.Chart.Result[0].Meta.RegularMarketPrice, nil
}

type Filter struct {
	Assets       []string `form:"symbols[]"`
	Trades       []string `form:"trade[]"`
	Tags         []string `form:"tags[]"`
	NeedsAllTags bool     `form:"needsAllTags"`
}

// loadSettings liest den globalen Settings-Datensatz (Pk = 1).
// Existiert noch keiner, wird ein Default mit Kontogröße 0 zurückgegeben.
func loadSettings(db *storm.DB) Settings {
	var s Settings
	if err := db.One("Pk", 1, &s); err != nil {
		return Settings{Pk: 1}
	}
	return s
}

// effectiveMarginTarget liefert den Ziel-Kontostatus für den Margin-Puffer.
// Nicht gesetzt (0) bedeutet Default 25 % (harter Stop-out).
func effectiveMarginTarget(s Settings) F32 {
	if s.MarginTargetPct <= 0 {
		return 25
	}
	return s.MarginTargetPct
}

// computeMarginBuffer berechnet kontoweit (Cross-Margin) die empfohlene Mindest-
// Einzahlung, damit der T212-Margin-Status auch dann über dem Ziel bleibt, wenn alle
// offenen Stops gleichzeitig greifen. Faktor = 50/(100−Ziel) (25 %→0,667, 45 %→0,909,
// 50 %→1,0). Jeder Trade trägt additiv Risk + Faktor·Margin_am_Stop bei; die Einzahlung
// muss zusätzlich die aktuell reservierte Entry-Margin decken (Untergrenze).
func computeMarginBuffer(trades []Trade, targetPct F32) MarginBufferSummary {
	factor := 50 / (100 - float64(targetPct))
	out := MarginBufferSummary{TargetPct: targetPct}
	for _, t := range trades {
		if t.Result != RESULT_NOTFINISHED {
			continue
		}
		out.OpenCount++
		risk := t.RiskLossAcct()
		marginEntry := t.MarginAcct()
		marginStop := t.MarginAtStopAcct()
		contrib := risk + F32(factor*float64(marginStop))

		out.TotalRisk += risk
		out.TotalMarginEntry += marginEntry
		out.TotalMarginStop += marginStop
		out.Items = append(out.Items, MarginBufferItem{
			Symbol: t.Symbol, Direction: t.Traded,
			Risk: risk, MarginEntry: marginEntry, MarginStop: marginStop, Contribution: contrib,
		})
	}
	target := out.TotalRisk + F32(factor*float64(out.TotalMarginStop))
	if out.TotalMarginEntry > target {
		out.RequiredDeposit = out.TotalMarginEntry
		out.FlooredByMargin = true
	} else {
		out.RequiredDeposit = target
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].Symbol < out.Items[j].Symbol })
	return out
}

func computeClassRisk(trades []Trade, assets []Asset, classes []AssetClass, accountSize F32) []ClassRiskSummary {
	symbolToClass := make(map[string]string, len(assets))
	for _, a := range assets {
		symbolToClass[a.Symbol] = a.AssetClass
	}

	limits := make(map[string]F32, len(classes))
	for _, c := range classes {
		limits[c.Title] = c.RiskLimit
	}

	classRisk := make(map[string]ClassRiskSummary)
	// Klassen mit gepflegtem Limit vorab anlegen, damit sie auch ohne offene
	// Trades (mit vollem freien Spielraum) erscheinen.
	for _, c := range classes {
		if c.RiskLimit > 0 {
			classRisk[c.Title] = ClassRiskSummary{Class: c.Title}
		}
	}
	for _, t := range trades {
		if t.Result != RESULT_NOTFINISHED {
			continue
		}
		class := symbolToClass[t.Symbol]
		if class == "" {
			class = "Unclassified"
		}
		s := classRisk[class]
		s.Class = class
		s.TotalRisk += t.RiskPercent(accountSize)
		s.TotalRiskAmount += t.RiskAmountAcct()
		s.TradeCount++
		classRisk[class] = s
	}

	result := make([]ClassRiskSummary, 0, len(classRisk))
	for _, v := range classRisk {
		if limit := limits[v.Class]; limit > 0 {
			v.HasLimit = true
			v.LimitPct = limit
			v.LimitAmount = limit * accountSize / 100
			v.FreePct = limit - v.TotalRisk
			v.FreeAmount = v.LimitAmount - v.TotalRiskAmount
		}
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Class < result[j].Class
	})
	return result
}

// computeSymbolRisk aggregiert das offene Risiko je Symbol und vergleicht es mit
// dem globalen Limit pro Asset (perAssetLimit in % vom Konto). Nur offene Trades.
// Das freie Risiko berücksichtigt zusätzlich das noch freie Risiko im Sektor
// (Asset-Klasse): es gilt der kleinere der beiden Werte (Sektor-Frei hat Vorrang,
// wenn es enger ist), da ein neuer Trade auf dem Asset auch den Sektor belastet.
func computeSymbolRisk(trades []Trade, assets []Asset, perAssetLimit, accountSize F32, classRisk []ClassRiskSummary) []SymbolRiskSummary {
	symbolToClass := make(map[string]string, len(assets))
	for _, a := range assets {
		symbolToClass[a.Symbol] = a.AssetClass
	}

	classByName := make(map[string]ClassRiskSummary, len(classRisk))
	for _, cr := range classRisk {
		classByName[cr.Class] = cr
	}

	symbolRisk := make(map[string]SymbolRiskSummary)
	for _, t := range trades {
		if t.Result != RESULT_NOTFINISHED {
			continue
		}
		s := symbolRisk[t.Symbol]
		s.Symbol = t.Symbol
		s.Class = symbolToClass[t.Symbol]
		s.TotalRisk += t.RiskPercent(accountSize)
		s.TotalRiskAmount += t.RiskAmountAcct()
		s.TradeCount++
		symbolRisk[t.Symbol] = s
	}

	result := make([]SymbolRiskSummary, 0, len(symbolRisk))
	for _, v := range symbolRisk {
		if perAssetLimit > 0 {
			v.HasLimit = true
			v.LimitPct = perAssetLimit
			v.LimitAmount = perAssetLimit * accountSize / 100
			v.FreePct = perAssetLimit - v.TotalRisk
			v.FreeAmount = v.LimitAmount - v.TotalRiskAmount

			// Sektor-Frei einbeziehen: ist im Sektor weniger frei als auf dem
			// Asset, hat der kleinere (Sektor-)Wert Vorrang.
			class := v.Class
			if class == "" {
				class = "Unclassified"
			}
			if cr, ok := classByName[class]; ok && cr.HasLimit {
				sectorFreePct := cr.LimitPct - cr.TotalRisk
				if sectorFreePct < v.FreePct {
					v.FreePct = sectorFreePct
					v.FreeAmount = cr.LimitAmount - cr.TotalRiskAmount
					v.SectorBinds = true
				}
			}
		}
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Symbol < result[j].Symbol
	})
	return result
}

// formRiskData baut die für das Eintrags-Formular benötigten Nachschlage-Maps
// (offenes Risiko je Symbol/Klasse, Symbol→Klasse, Klassen-Limits). Die Werte
// sind float64, damit Alpine im Browser damit rechnen kann (F32 würde wegen
// MarshalText als String serialisiert). excludePk schließt einen Trade aus der
// Summe aus (für /edit, damit der bearbeitete Eintrag nicht doppelt zählt).
func formRiskData(trades []Trade, assets []Asset, classes []AssetClass, accountSize F32, excludePk int) (symbolRisk, classRisk map[string]map[string]float64, symbolClass map[string]string, classLimits map[string]float64) {
	symbolClass = make(map[string]string, len(assets))
	for _, a := range assets {
		symbolClass[a.Symbol] = a.AssetClass
	}

	classLimits = make(map[string]float64, len(classes))
	for _, c := range classes {
		classLimits[c.Title] = float64(c.RiskLimit)
	}

	symbolRisk = make(map[string]map[string]float64)
	classRisk = make(map[string]map[string]float64)
	for _, t := range trades {
		if t.Result != RESULT_NOTFINISHED || t.Pk == excludePk {
			continue
		}
		pct := float64(t.RiskPercent(accountSize))
		amount := float64(t.RiskAmountAcct())

		s := symbolRisk[t.Symbol]
		if s == nil {
			s = map[string]float64{}
		}
		s["pct"] += pct
		s["amount"] += amount
		symbolRisk[t.Symbol] = s

		class := symbolClass[t.Symbol]
		if class == "" {
			class = "Unclassified"
		}
		c := classRisk[class]
		if c == nil {
			c = map[string]float64{}
		}
		c["pct"] += pct
		c["amount"] += amount
		classRisk[class] = c
	}
	return
}

// computeStats berechnet Kennzahlen über die übergebene (bereits gefilterte
// bzw. vollständige) Trade-Menge für die Kennzahlen-Leiste der Übersicht.
func computeStats(trades []Trade, accountSize F32) StatsSummary {
	var s StatsSummary
	s.Total = len(trades)
	for _, t := range trades {
		switch t.Result {
		case RESULT_WIN:
			s.Wins++
		case RESULT_LOSS:
			s.Losses++
		case RESULT_BREAKEVEN:
			s.BreakEven++
		case RESULT_NOTFINISHED:
			s.Open++
			s.OpenRisk += t.RiskAmountAcct()
			s.OpenRiskPct += t.RiskPercent(accountSize)
		case RESULT_SKIP:
			s.Skips++
		}
	}
	if decided := s.Wins + s.Losses; decided > 0 {
		s.WinRate = F32(float64(s.Wins) / float64(decided) * 100)
	}
	return s
}

// flashMessage übersetzt einen kurzen Flash-Code (aus dem ?flash=-Query-Param)
// in einen anzuzeigenden Text und einen Bootstrap-Alert-Typ. Unbekannte Codes
// liefern leere Werte, sodass keine Meldung gerendert wird.
func flashMessage(code string) (msg, typ string) {
	switch code {
	case "trade-saved":
		return "Eintrag gespeichert.", "success"
	case "trade-updated":
		return "Eintrag aktualisiert.", "success"
	case "trade-deleted":
		return "Eintrag gelöscht.", "success"
	case "settings-saved":
		return "Einstellungen gespeichert.", "success"
	case "reconcile-needed":
		return "Mindestens ein Exit ist noch nicht verrechnet. Hake ihn unten ab, um den Betrag auf deinen Kontostand zu buchen.", "warning"
	default:
		return "", ""
	}
}

func CreateRoutes(db *storm.DB, r *gin.Engine) {

	r.GET("/", func(c *gin.Context) {
		var trades []Trade
		err := db.All(&trades, storm.Reverse())
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var tags []Tag
		err = db.All(&tags, storm.Reverse())
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var stringTags []string
		for _, tag := range tags {
			stringTags = append(stringTags, tag.Title)
		}

		var assets []Asset
		err = db.All(&assets)

		var symbols []string
		for _, symbol := range assets {
			symbols = append(symbols, symbol.Symbol)
		}

		var classes []AssetClass
		err = db.All(&classes)

		settings := loadSettings(db)

		flash, flashType := flashMessage(c.Query("flash"))

		classRisk := computeClassRisk(trades, assets, classes, settings.AccountSize)
		marginTarget := effectiveMarginTarget(settings)

		c.HTML(200, "index", gin.H{
			"tags":              stringTags,
			"symbols":           symbols,
			"trades":            trades,
			"classRisk":         classRisk,
			"symbolRisk":        computeSymbolRisk(trades, assets, settings.PerAssetRiskLimit, settings.AccountSize, classRisk),
			"perAssetRiskLimit": settings.PerAssetRiskLimit,
			"stats":             computeStats(trades, settings.AccountSize),
			"marginBuffer":      computeMarginBuffer(trades, marginTarget),
			"marginTargetPct":   marginTarget,
			"accountSize":       settings.AccountSize,
			"flash":             flash,
			"flashType":         flashType,
		})
	})

	r.POST("/", func(c *gin.Context) {
		var filter Filter
		err := c.Bind(&filter)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var trades []Trade
		err = db.All(&trades, storm.Reverse())
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var filteredTrades []Trade

	TRADES:
		for _, trade := range trades {
			// Leere Auswahl in einer Kategorie bedeutet "egal" und schränkt nicht ein.
			if len(filter.Trades) > 0 && !slices.Contains(filter.Trades, string(trade.Traded)) {
				continue
			}

			if len(filter.Assets) > 0 && !slices.Contains(filter.Assets, trade.Symbol) {
				continue
			}

			if len(filter.Tags) > 0 {
				if filter.NeedsAllTags {
					// Muss ALLE gewählten Tags haben.
					for _, tag := range filter.Tags {
						if !slices.Contains(trade.Tags, tag) {
							continue TRADES
						}
					}
				} else {
					// Muss IRGENDEINEN der gewählten Tags haben.
					hasAny := false
					for _, tag := range filter.Tags {
						if slices.Contains(trade.Tags, tag) {
							hasAny = true
							break
						}
					}
					if !hasAny {
						continue TRADES
					}
				}
			}

			filteredTrades = append(filteredTrades, trade)
		}

		var tags []Tag
		err = db.All(&tags, storm.Reverse())
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var stringTags []string
		for _, tag := range tags {
			stringTags = append(stringTags, tag.Title)
		}

		var assets []Asset
		err = db.All(&assets)

		var symbols []string
		for _, symbol := range assets {
			symbols = append(symbols, symbol.Symbol)
		}

		var classes []AssetClass
		err = db.All(&classes)

		settings := loadSettings(db)

		classRisk := computeClassRisk(trades, assets, classes, settings.AccountSize)
		marginTarget := effectiveMarginTarget(settings)

		c.HTML(200, "index", gin.H{
			"tags":              stringTags,
			"symbols":           symbols,
			"trades":            filteredTrades,
			"filter":            filter,
			"classRisk":         classRisk,
			"symbolRisk":        computeSymbolRisk(trades, assets, settings.PerAssetRiskLimit, settings.AccountSize, classRisk),
			"perAssetRiskLimit": settings.PerAssetRiskLimit,
			"stats":             computeStats(filteredTrades, settings.AccountSize),
			"marginBuffer":      computeMarginBuffer(filteredTrades, marginTarget),
			"marginTargetPct":   marginTarget,
			"accountSize":       settings.AccountSize,
		})
	})

	r.GET("/add", func(c *gin.Context) {
		var tags []Tag
		err := db.All(&tags, storm.Reverse())
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var stringTags []string
		for _, tag := range tags {
			stringTags = append(stringTags, tag.Title)
		}

		var assets []Asset
		err = db.All(&assets)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var symbols []string
		for _, symbol := range assets {
			symbols = append(symbols, symbol.Symbol)
		}

		var assetClasses []AssetClass
		err = db.All(&assetClasses)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var stringClasses []string
		for _, cls := range assetClasses {
			stringClasses = append(stringClasses, cls.Title)
		}

		var trades []Trade
		err = db.All(&trades)

		settings := loadSettings(db)
		symbolRisk, classRisk, symbolClass, classLimits := formRiskData(trades, assets, assetClasses, settings.AccountSize, 0)

		c.HTML(200, "add", gin.H{
			"tags":              stringTags,
			"symbols":           symbols,
			"assetClasses":      stringClasses,
			"accountSize":       settings.AccountSize,
			"perAssetRiskLimit": settings.PerAssetRiskLimit,
			"marginTargetPct":   effectiveMarginTarget(settings),
			"symbolRisk":        symbolRisk,
			"classRisk":         classRisk,
			"symbolClass":       symbolClass,
			"classLimits":       classLimits,
		})
	})

	// /fx liefert den aktuellen Wechselkurs (Kontowährung je Trade-Währung) zur
	// automatischen Vorbefüllung im Eintrags-Formular. Default USD→EUR.
	r.GET("/fx", func(c *gin.Context) {
		pair := c.DefaultQuery("pair", "USDEUR=X")
		rate, err := fetchFxRate(pair)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"pair": pair, "rate": rate})
	})

	r.GET("/settings", func(c *gin.Context) {
		var tags []Tag
		err := db.All(&tags, storm.Reverse())
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var stringTags []string
		for _, tag := range tags {
			stringTags = append(stringTags, tag.Title)
		}

		var assets []Asset
		err = db.All(&assets)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var symbols []string
		for _, symbol := range assets {
			symbols = append(symbols, symbol.Symbol)
		}

		var assetClasses []AssetClass
		err = db.All(&assetClasses)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var stringClasses []string
		for _, cls := range assetClasses {
			stringClasses = append(stringClasses, cls.Title)
		}

		flash, flashType := flashMessage(c.Query("flash"))

		settings := loadSettings(db)

		c.HTML(200, "settings", gin.H{
			"tags":              stringTags,
			"symbols":           symbols,
			"assetClasses":      stringClasses,
			"assetClassLimits":  assetClasses,
			"accountSize":       settings.AccountSize,
			"perAssetRiskLimit": settings.PerAssetRiskLimit,
			"marginTargetPct":   effectiveMarginTarget(settings),
			"flash":             flash,
			"flashType":         flashType,
		})
	})

	r.GET("/edit/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 0)

		var trade Trade

		err = db.One("Pk", id, &trade)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var tags []Tag
		err = db.All(&tags, storm.Reverse())
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var stringTags []string
		for _, tag := range tags {
			stringTags = append(stringTags, tag.Title)
		}

		var assets []Asset
		err = db.All(&assets)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var symbols []string
		for _, symbol := range assets {
			symbols = append(symbols, symbol.Symbol)
		}

		var assetClasses []AssetClass
		err = db.All(&assetClasses)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var stringClasses []string
		for _, cls := range assetClasses {
			stringClasses = append(stringClasses, cls.Title)
		}

		var trades []Trade
		err = db.All(&trades)

		settings := loadSettings(db)
		// Den bearbeiteten Trade aus der Summe ausschließen, damit er beim
		// Vergleich nicht doppelt zählt (die Live-Vorschau addiert ihn neu).
		symbolRisk, classRisk, symbolClass, classLimits := formRiskData(trades, assets, assetClasses, settings.AccountSize, trade.Pk)

		flash, flashType := flashMessage(c.Query("flash"))

		c.HTML(200, "add", gin.H{
			"tags":              stringTags,
			"symbols":           symbols,
			"assetClasses":      stringClasses,
			"trade":             trade,
			"accountSize":       settings.AccountSize,
			"perAssetRiskLimit": settings.PerAssetRiskLimit,
			"marginTargetPct":   effectiveMarginTarget(settings),
			"symbolRisk":        symbolRisk,
			"classRisk":         classRisk,
			"symbolClass":       symbolClass,
			"classLimits":       classLimits,
			"flash":             flash,
			"flashType":         flashType,
		})
	})

	r.POST("/settings", func(c *gin.Context) {
		settings := loadSettings(db)

		if err := c.ShouldBind(&settings); err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
		settings.Pk = 1

		if err := db.Save(&settings); err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		if c.GetHeader("HX-Request") != "" {
			c.JSON(200, settings)
		} else {
			c.Redirect(302, "/settings?flash=settings-saved")
		}
	})

	r.DELETE("/delete/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 0)

		var trade Trade
		err = db.One("Pk", id, &trade)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		err = db.DeleteStruct(&trade)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		c.Redirect(302, "/?flash=trade-deleted")
	})

	r.POST("/add-tag", func(c *gin.Context) {
		var tag Tag

		err := c.ShouldBind(&tag)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		err = db.Save(&tag)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var tags []Tag
		err = db.All(&tags, storm.Reverse())

		var stringTags []string
		for _, tag := range tags {
			stringTags = append(stringTags, tag.Title)
		}

		if c.GetHeader("HX-Request") != "" {
			c.JSON(200, stringTags)
		} else {
			c.Redirect(302, "/settings")
		}
	})

	r.POST("/remove-tag", func(c *gin.Context) {
		var tag Tag

		err := c.ShouldBind(&tag)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		err = db.DeleteStruct(&tag)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var tags []Tag
		err = db.All(&tags, storm.Reverse())

		var stringTags []string
		for _, tag := range tags {
			stringTags = append(stringTags, tag.Title)
		}

		if c.GetHeader("HX-Request") != "" {
			c.JSON(200, stringTags)
		} else {
			c.Redirect(302, "/settings")
		}

	})

	r.POST("/add-symbol", func(c *gin.Context) {
		var asset Asset

		err := c.ShouldBind(&asset)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		err = db.Save(&asset)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var assets []Asset
		err = db.All(&assets)

		var symbols []string
		for _, symbol := range assets {
			symbols = append(symbols, symbol.Symbol)
		}

		if c.GetHeader("HX-Request") != "" {
			c.JSON(200, symbols)
		} else {
			c.Redirect(302, "/settings")
		}
	})

	r.POST("/remove-symbol", func(c *gin.Context) {
		var asset Asset

		err := c.ShouldBind(&asset)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		err = db.DeleteStruct(&asset)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var assets []Asset
		err = db.All(&assets)

		var symbols []string
		for _, symbol := range assets {
			symbols = append(symbols, symbol.Symbol)
		}

		if c.GetHeader("HX-Request") != "" {
			c.JSON(200, symbols)
		} else {
			c.Redirect(302, "/settings")
		}

	})

	r.POST("/add-asset-class", func(c *gin.Context) {
		var cls AssetClass

		err := c.ShouldBind(&cls)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		err = db.Save(&cls)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var assetClasses []AssetClass
		err = db.All(&assetClasses)

		var stringClasses []string
		for _, ac := range assetClasses {
			stringClasses = append(stringClasses, ac.Title)
		}

		if c.GetHeader("HX-Request") != "" {
			c.JSON(200, stringClasses)
		} else {
			c.Redirect(302, "/settings")
		}
	})

	r.POST("/remove-asset-class", func(c *gin.Context) {
		var cls AssetClass

		err := c.ShouldBind(&cls)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		err = db.DeleteStruct(&cls)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		var assetClasses []AssetClass
		err = db.All(&assetClasses)

		var stringClasses []string
		for _, ac := range assetClasses {
			stringClasses = append(stringClasses, ac.Title)
		}

		if c.GetHeader("HX-Request") != "" {
			c.JSON(200, stringClasses)
		} else {
			c.Redirect(302, "/settings")
		}
	})

	r.POST("/set-class-limit", func(c *gin.Context) {
		var cls AssetClass

		// Title (name="Title") und RiskLimit (form:"riskLimit") werden gebunden.
		// AssetClass hält nur diese beiden Felder, daher ist db.Save ein
		// vollständiges, korrektes Überschreiben des Datensatzes.
		if err := c.ShouldBind(&cls); err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		if err := db.Save(&cls); err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		c.Redirect(302, "/settings?flash=settings-saved")
	})

	r.POST("/insert", func(c *gin.Context) {
		var trade Trade

		err := c.ShouldBind(&trade)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		isUpdate := trade.Pk != 0

		if exitsJSON := c.PostForm("exitsJSON"); exitsJSON != "" {
			json.Unmarshal([]byte(exitsJSON), &trade.Exits)
		}

		// CreatedAt ist nicht im Formular (form:"-") und nach dem Bind leer. Beim
		// Bearbeiten das vorhandene Datum erhalten, nur bei neuen Trades neu setzen.
		// bookedBefore = bereits verrechnete Beträge im gespeicherten Stand, damit
		// beim Speichern nur die Differenz auf den Kontostand gebucht wird.
		var bookedBefore F32
		if isUpdate {
			var existing Trade
			if err := db.One("Pk", trade.Pk, &existing); err == nil {
				trade.CreatedAt = existing.CreatedAt
				bookedBefore = existing.SettledTotal()
			}
		} else {
			trade.CreatedAt = time.Now()
		}

		err = db.Save(&trade)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		filenamePrefix := strconv.Itoa(trade.Pk) + "-" + time.Now().Format("2006-01-02T15:04:05")

		form, err := c.MultipartForm()
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		log.Println(trade.Screenshots)

		files := form.File["files"]
		for _, file := range files {
			log.Println(file.Filename)
			filename := filenamePrefix + "-" + file.Filename

			err := c.SaveUploadedFile(file, "./uploads/"+filename)
			if err != nil {
				c.String(http.StatusBadRequest, "error saving file: %s", file.Filename)
				return
			}

			trade.Screenshots = append(trade.Screenshots, filename)
		}

		err = db.Save(&trade)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		// Verrechnen: nur die Differenz der verrechneten Exit-Beträge gegenüber dem
		// vorherigen Stand auf den Kontostand buchen. Neu abgehakte Exits erhöhen,
		// wieder abgewählte verringern den Kontostand – der Trade selbst (Entry,
		// Stückzahl, Risiko) bleibt davon unberührt.
		if delta := trade.SettledTotal() - bookedBefore; delta != 0 {
			settings := loadSettings(db)
			settings.Pk = 1
			settings.AccountSize += delta
			if err := db.Save(&settings); err != nil {
				c.String(http.StatusBadRequest, err.Error())
				return
			}
		}

		// Stups: gibt es noch einen Exit, der nicht verrechnet wurde, zurück auf die
		// Edit-Seite mit Hinweis – der Nutzer wird genötigt, aber nicht gezwungen.
		if trade.NeedsReconcile() {
			c.Redirect(302, "/edit/"+strconv.Itoa(trade.Pk)+"?flash=reconcile-needed")
			return
		}

		if isUpdate {
			c.Redirect(302, "/?flash=trade-updated")
		} else {
			c.Redirect(302, "/?flash=trade-saved")
		}
	})
}
