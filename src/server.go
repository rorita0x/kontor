package src

import (
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/asdine/storm/v3"
	"github.com/gin-gonic/gin"
)

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

func computeClassRisk(trades []Trade, assets []Asset, accountSize F32) []ClassRiskSummary {
	symbolToClass := make(map[string]string, len(assets))
	for _, a := range assets {
		symbolToClass[a.Symbol] = a.AssetClass
	}

	classRisk := make(map[string]ClassRiskSummary)
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
		s.TotalRiskAmount += t.RiskAmount()
		s.TradeCount++
		classRisk[class] = s
	}

	result := make([]ClassRiskSummary, 0, len(classRisk))
	for _, v := range classRisk {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Class < result[j].Class
	})
	return result
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

		settings := loadSettings(db)

		flash, flashType := flashMessage(c.Query("flash"))

		c.HTML(200, "index", gin.H{
			"tags":        stringTags,
			"symbols":     symbols,
			"trades":      trades,
			"classRisk":   computeClassRisk(trades, assets, settings.AccountSize),
			"accountSize": settings.AccountSize,
			"flash":       flash,
			"flashType":   flashType,
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

		settings := loadSettings(db)

		c.HTML(200, "index", gin.H{
			"tags":        stringTags,
			"symbols":     symbols,
			"trades":      filteredTrades,
			"filter":      filter,
			"classRisk":   computeClassRisk(trades, assets, settings.AccountSize),
			"accountSize": settings.AccountSize,
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

		c.HTML(200, "add", gin.H{
			"tags":         stringTags,
			"symbols":      symbols,
			"assetClasses": stringClasses,
			"accountSize":  loadSettings(db).AccountSize,
		})
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

		c.HTML(200, "settings", gin.H{
			"tags":         stringTags,
			"symbols":      symbols,
			"assetClasses": stringClasses,
			"accountSize":  loadSettings(db).AccountSize,
			"flash":        flash,
			"flashType":    flashType,
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

		c.HTML(200, "add", gin.H{
			"tags":         stringTags,
			"symbols":      symbols,
			"assetClasses": stringClasses,
			"trade":        trade,
			"accountSize":  loadSettings(db).AccountSize,
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
		if isUpdate {
			var existing Trade
			if err := db.One("Pk", trade.Pk, &existing); err == nil {
				trade.CreatedAt = existing.CreatedAt
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

		if isUpdate {
			c.Redirect(302, "/?flash=trade-updated")
		} else {
			c.Redirect(302, "/?flash=trade-saved")
		}
	})
}
