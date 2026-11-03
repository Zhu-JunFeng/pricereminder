// Package gold reads the Au99.99 public quotation without substituting the
// fetch time for the upstream quotation timestamp.
package gold

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const Symbol = "SGE_AU9999"
const Endpoint = "https://hq.sinajs.cn/list=gds_AU9999"

var beijing = time.FixedZone("Asia/Shanghai", 8*60*60)

type Quote struct {
	Price         string `json:"price"`
	PreviousClose string `json:"previousClose"`
	EventTime     int64  `json:"eventTime"`
	ReceivedAt    int64  `json:"receivedAt"`
}

// Parse accepts only the expected instrument and complete, positive prices.
// The trailing display name is GBK on Sina; all consumed fields are ASCII.
func Parse(body []byte, received time.Time) (Quote, error) {
	text := strings.TrimSpace(string(body))
	const prefix = `var hq_str_gds_AU9999="`
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, `";`) {
		return Quote{}, fmt.Errorf("unexpected Sina Au9999 response")
	}
	fields := strings.Split(strings.TrimSuffix(strings.TrimPrefix(text, prefix), `";`), ",")
	if len(fields) != 14 {
		return Quote{}, fmt.Errorf("unexpected Sina field count: %d", len(fields))
	}
	price, err := decimal.NewFromString(fields[0])
	if err != nil || !price.IsPositive() {
		return Quote{}, fmt.Errorf("invalid gold price")
	}
	closePrice, err := decimal.NewFromString(fields[7])
	previousClose := ""
	if err == nil && closePrice.IsPositive() {
		previousClose = closePrice.String()
	}
	stamp, err := time.ParseInLocation("2006-01-02 15:04:05", fields[12]+" "+fields[6], beijing)
	if err != nil {
		return Quote{}, fmt.Errorf("invalid source timestamp: %w", err)
	}
	if stamp.After(received.Add(5 * time.Second)) {
		return Quote{}, fmt.Errorf("source timestamp in future")
	}
	return Quote{Price: price.String(), PreviousClose: previousClose, EventTime: stamp.UnixMilli(), ReceivedAt: received.UnixMilli()}, nil
}

func Fetch(ctx context.Context, client *http.Client) (Quote, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Endpoint, nil)
	if err != nil {
		return Quote{}, err
	}
	req.Header.Set("Referer", "https://finance.sina.com.cn/")
	req.Header.Set("User-Agent", "PriceReminder/GoldSourceValidation")
	response, err := client.Do(req)
	if err != nil {
		return Quote{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Quote{}, fmt.Errorf("Sina HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 16*1024))
	if err != nil {
		return Quote{}, err
	}
	return Parse(body, time.Now())
}
