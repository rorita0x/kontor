package src

import (
	"strconv"
	"time"
)

type Exit struct {
	Date     string `json:"date"`
	Price    F32    `json:"price"`
	Quantity F32    `json:"qty"`
}

type Traded string

type TradeResult string

const (
	TRADED_SKIP  Traded = "skip"
	TRADED_LONG         = "long"
	TRADED_SHORT        = "short"
)

const (
	RESULT_NOTFINISHED TradeResult = "not finished"
	RESULT_SKIP                    = "skip"
	RESULT_LOSS                    = "loss"
	RESULT_BREAKEVEN               = "break-even"
	RESULT_WIN                     = "win"
)

func (r TradeResult) IsCorrect() bool {
	return r == RESULT_WIN
}

type F32 float32

func (f *F32) UnmarshalText(b []byte) error {
	v, err := strconv.ParseFloat(string(b), 32)
	if err != nil {
		return err
	}
	*f = F32(v)
	return nil
}

func (f F32) MarshalText() ([]byte, error) {
	s := strconv.FormatFloat(float64(f), 'f', 2, 32)
	return []byte(s), nil
}

func (f F32) String() string {
	return strconv.FormatFloat(float64(f), 'f', 2, 32)
}

type Trade struct {
	Pk        int       `storm:"id,increment" json:"id" form:"id" uri:"id"`
	CreatedAt time.Time `storm:"index" json:"createdAt" form:"-" uri:"-" binding:"-"`

	Symbol      string   `form:"symbol" json:"symbol" form:"symbol" binding:"required"`
	Screenshots []string `form:"screenshots[]" json:"screenshots"`
	Tags        []string `form:"tags[]" json:"tags"`

	Notes string `form:"notes" json:"notes"`

	Traded Traded      `form:"traded" binding:"required"`
	Result TradeResult `form:"result" binding:"required"`

	EntryPrice F32    `form:"entryPrice" json:"entryPrice"`
	StopLoss   F32    `form:"stopLoss" json:"stopLoss"`
	Exits      []Exit `json:"exits"`
}

func (t Trade) RiskFromSL() F32 {
	if t.EntryPrice <= 0 || t.StopLoss <= 0 {
		return 0
	}
	diff := t.EntryPrice - t.StopLoss
	if diff < 0 {
		diff = -diff
	}
	return diff / t.EntryPrice * 100
}

type Tag struct {
	Title string `storm:"id"`
}

type Asset struct {
	Symbol     string `storm:"id"`
	AssetClass string `form:"assetClass" json:"assetClass"`
}

type AssetClass struct {
	Title string `storm:"id"`
}

type ClassRiskSummary struct {
	Class      string
	TotalRisk  F32
	TradeCount int
}
