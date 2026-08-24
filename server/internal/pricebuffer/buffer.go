package pricebuffer

import (
	"sort"
	"sync"

	"pricereminder/server/internal/domain"
)

const RetentionMillis int64 = 60 * 60 * 1000

type Buffer struct {
	mu     sync.RWMutex
	points map[string][]domain.PricePoint
}

func New() *Buffer {
	return &Buffer{points: make(map[string][]domain.PricePoint)}
}

func (b *Buffer) Add(point domain.PricePoint) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	items := b.points[point.Symbol]
	if len(items) > 0 {
		last := items[len(items)-1]
		if point.EventTime < last.EventTime {
			return false
		}
		if point.EventTime/1000 == last.EventTime/1000 {
			items[len(items)-1] = point
			b.points[point.Symbol] = items
			return true
		}
	}

	items = append(items, point)
	cutoff := point.EventTime - RetentionMillis
	first := sort.Search(len(items), func(i int) bool { return items[i].EventTime >= cutoff })
	if first > 0 {
		items = append([]domain.PricePoint(nil), items[first:]...)
	}
	b.points[point.Symbol] = items
	return true
}

func (b *Buffer) AtOrBefore(symbol string, eventTime int64) (domain.PricePoint, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	items := b.points[symbol]
	index := sort.Search(len(items), func(i int) bool { return items[i].EventTime > eventTime })
	if index == 0 {
		return domain.PricePoint{}, false
	}
	return items[index-1], true
}

func (b *Buffer) Range(symbol string, after int64) []domain.PricePoint {
	b.mu.RLock()
	defer b.mu.RUnlock()
	items := b.points[symbol]
	first := sort.Search(len(items), func(i int) bool { return items[i].EventTime > after })
	result := make([]domain.PricePoint, len(items)-first)
	copy(result, items[first:])
	for i := range result {
		result[i].Replay = true
	}
	return result
}

func (b *Buffer) Latest(symbol string) (domain.PricePoint, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	items := b.points[symbol]
	if len(items) == 0 {
		return domain.PricePoint{}, false
	}
	return items[len(items)-1], true
}

func (b *Buffer) Earliest(symbol string) (domain.PricePoint, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	items := b.points[symbol]
	if len(items) == 0 {
		return domain.PricePoint{}, false
	}
	return items[0], true
}

func (b *Buffer) Status() (symbols int, latestEventTime int64) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, items := range b.points {
		if len(items) == 0 {
			continue
		}
		symbols++
		if items[len(items)-1].EventTime > latestEventTime {
			latestEventTime = items[len(items)-1].EventTime
		}
	}
	return symbols, latestEventTime
}
