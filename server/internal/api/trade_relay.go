package api

import (
	"net/http"
	"pricereminder/server/internal/traderelay"
)

func (s *Server) TradeRelayHandler(relay *traderelay.Relay) http.Handler {
	s.activeSymbols = relay.ActiveSymbols
	return s.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relay.Serve(w, r, currentDevice(r.Context()).ID.String(), s.allowedOrigin)
	}))
}
