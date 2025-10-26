package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/asdine/storm/v3"
	"github.com/gin-contrib/multitemplate"
	"github.com/gin-gonic/gin"
	"github.com/spf13/afero"
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
	jailedFs := afero.NewBasePathFs(osFs, "./uploads/")
	assetsFs := afero.NewBasePathFs(osFs, "./assets/")

	httpFS := afero.NewHttpFs(jailedFs)
	fileServer := http.FileServer(httpFS.Dir("/"))

	assetsHttpFS := afero.NewHttpFs(assetsFs)
	assetsFileServer := http.FileServer(assetsHttpFS.Dir("/"))

	for _, tag := range TAGS {
		err = db.Save(&Tag{tag})
		if err != nil {
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

	r.GET("/", func(c *gin.Context) {
		var trades []Trade
		err = db.All(&trades, storm.Reverse())

		var tags []Tag
		err = db.All(&tags, storm.Reverse())

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

		c.HTML(200, "index", gin.H{
			"tags":    stringTags,
			"symbols": symbols,
			"trades":  trades,
		})
	})

	r.GET("/add", func(c *gin.Context) {
		var tags []Tag
		err = db.All(&tags, storm.Reverse())

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

		c.HTML(200, "add", gin.H{
			"tags":    stringTags,
			"symbols": symbols,
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

		c.HTML(200, "add", gin.H{
			"tags":    stringTags,
			"symbols": symbols,
			"trade":   trade,
		})
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
			c.Redirect(302, "/add")
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
			c.Redirect(302, "/add")
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
			c.Redirect(302, "/add")
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
			c.Redirect(302, "/add")
		}

	})

	r.POST("/insert", func(c *gin.Context) {
		var trade Trade

		err := c.ShouldBind(&trade)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}

		trade.CreatedAt = time.Now()

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

		// c.String(http.StatusOK, fmt.Sprintf("%d files uploaded!", len(files)))
		c.Redirect(302, "/")
	})

	err = r.Run("127.0.0.1:18596")
	if err != nil {
		log.Fatal(err)
	}
}
