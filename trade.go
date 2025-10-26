package main

import (
	"strconv"
	"time"
)

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
	RESULT_TP1                     = "tp1"
	RESULT_TP2                     = "tp2"
	RESULT_TP3                     = "tp3"
)

func (r TradeResult) IsCorrect() bool {
	switch r {
	case RESULT_TP1, RESULT_TP2, RESULT_TP3:
		return true
	default:
		return false
	}
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
	Screenshots []string `form:"-" binding:"-"`
	Tags        []string `form:"tags[]" binding:"required"`

	Traded Traded      `form:"traded" binding:"required"`
	Result TradeResult `form:"result" binding:"required"`

	Risk F32 `form:"risk"`

	TP1      string `form:"tp1"`
	TP1Ratio F32    `form:"tp1Ratio"`

	TP2      string `form:"tp2"`
	TP2Ratio F32    `form:"tp2Ratio"`

	TP3      string `form:"tp3"`
	TP3Ratio F32    `form:"tp3Ratio"`
}

type Tag struct {
	Title string `storm:"id"`
}

type Asset struct {
	Symbol string `storm:"id"`
}
