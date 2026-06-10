package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"reflect"

	"github.com/asdine/storm/v3"
	"github.com/gin-contrib/multitemplate"
	"github.com/gin-gonic/gin"
	"github.com/spf13/afero"
	"rorita.moe/trading-db/src"
)

func toJSONFuncMap() template.FuncMap {
	return template.FuncMap{
		"toJSON": func(v interface{}) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("{}")
			}
			buf := new(bytes.Buffer)
			json.HTMLEscape(buf, b)
			return template.JS(buf.String())
		},
		"in": func(slice interface{}, val interface{}) bool {
			s := reflect.ValueOf(slice)
			if s.Kind() != reflect.Slice && s.Kind() != reflect.Array {
				return false
			}
			for i := 0; i < s.Len(); i++ {
				if reflect.DeepEqual(s.Index(i).Interface(), val) {
					return true
				}
			}
			return false
		},
	}
}

func loadTemplates() multitemplate.Renderer {
	r := multitemplate.NewRenderer()

	funcs := toJSONFuncMap()

	r.AddFromFilesFuncs("index", funcs, "templates/base.html", "templates/index.html")
	r.AddFromFilesFuncs("add", funcs, "templates/base.html", "templates/add.html")
	r.AddFromFilesFuncs("tags", funcs, "templates/tags.html")

	return r
}

var ASSET_CLASSES = [...]string{"Crypto", "Commodities", "Forex", "Indices", "Metals", "Stocks"}

var TAGS = [...]string{
	"LSOB",
	"GUSS",
	"21 EMA",
	"50 EMA",
	"200 EMA",
	"Chop",
	"Liquidity Sweep",
	"Sauber",
	"Unsauber",
	"RSI Neckline Break Divergence",
	"R:R",
	"Break-Even",
	"Cycle Low",
	"Half-Cycle Low",
	"Weekly Cycle Low",
	"Left Translated",
	"Right Translated",
}

func main() {
	db, err := storm.Open("trading.db")
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("warning: failed to close db: %v", err)
		}
	}()

	osFs := afero.NewOsFs()
	uploadsFs := afero.NewBasePathFs(osFs, "./uploads/")
	assetsFs := afero.NewBasePathFs(osFs, "./assets/")

	httpFS := afero.NewHttpFs(uploadsFs)
	fileServer := http.FileServer(httpFS.Dir("/"))

	assetsHttpFS := afero.NewHttpFs(assetsFs)
	assetsFileServer := http.FileServer(assetsHttpFS.Dir("/"))

	for _, tag := range TAGS {
		err = db.Save(&src.Tag{Title: tag})
		if err != nil {
			log.Fatal(err)
		}
	}

	for _, cls := range ASSET_CLASSES {
		err = db.Save(&src.AssetClass{Title: cls})
		if err != nil {
			log.Fatal(err)
		}
	}

	// Globalen Settings-Datensatz (Pk = 1) anlegen, falls noch keiner existiert.
	var settings src.Settings
	if err = db.One("Pk", 1, &settings); err != nil {
		if err = db.Save(&src.Settings{Pk: 1}); err != nil {
			log.Fatal(err)
		}
	}

	r := gin.Default()

	r.HTMLRender = loadTemplates()
	r.MaxMultipartMemory = 64 << 20 // 64 MiB

	r.GET("/assets/*filepath", func(c *gin.Context) {
		c.Request.URL.Path = c.Param("filepath") // sync Gin param to HTTP path
		assetsFileServer.ServeHTTP(c.Writer, c.Request)
	})

	r.GET("/files/*filepath", func(c *gin.Context) {
		c.Request.URL.Path = c.Param("filepath") // sync Gin param to HTTP path
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	src.CreateRoutes(db, r)

	err = r.Run("127.0.0.1:18596")
	if err != nil {
		log.Fatal(err)
	}
}
