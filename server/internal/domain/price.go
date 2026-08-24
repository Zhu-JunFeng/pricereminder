package domain

import "github.com/shopspring/decimal"

type PricePoint struct {
	Symbol    string          `json:"symbol"`
	Price     decimal.Decimal `json:"-"`
	PriceText string          `json:"price"`
	EventTime int64           `json:"eventTime"`
	Replay    bool            `json:"replay"`
}

func NewPricePoint(symbol, price string, eventTime int64) (PricePoint, error) {
	value, err := decimal.NewFromString(price)
	if err != nil {
		return PricePoint{}, err
	}
	return PricePoint{Symbol: symbol, Price: value, PriceText: price, EventTime: eventTime}, nil
}
