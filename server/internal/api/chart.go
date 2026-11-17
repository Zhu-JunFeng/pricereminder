package api

import (
	"net/http"

	"pricereminder/server/internal/chart"
	"pricereminder/server/internal/gold"
)

func (s *Server) ChartHandler(history *chart.Service) http.Handler {
	return s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol != gold.Symbol && !s.catalog.ContainsSupportedQuote(symbol) {
			writeError(w, http.StatusBadRequest, "unsupported_symbol", "不支持该行情品种")
			return
		}
		data, err := history.Get(r.Context(), symbol)
		if err != nil {
			s.logger.Warn("chart history unavailable", "symbol", symbol, "error", err)
			writeError(w, http.StatusServiceUnavailable, "chart_unavailable", "走势暂不可用")
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, data)
	}))
}
