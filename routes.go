package main

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bugsnag/bugsnag-go/errors"
	"github.com/dgrijalva/jwt-go"
	"github.com/dimfeld/httptreemux"
	"github.com/unrolled/render"
	"github.com/usmbest/ocean.one/cache"
	"github.com/usmbest/ocean.one/config"
	"github.com/usmbest/ocean.one/persistence"
)

type R struct{}

func NewRouter() *httptreemux.TreeMux {
	router, impl := httptreemux.New(), &R{}
	router.GET("/brokers", impl.brokers)
	router.GET("/markets/:id/ticker", impl.marketTicker)
	router.GET("/markets/:id/book", impl.marketBook)
	router.GET("/markets/:id/trades", impl.marketTrades)
	router.GET("/orders", impl.orders)
	router.GET("/orders/:id", impl.order)
	router.POST("/tokens", impl.tokens)
	registerHanders(router)
	return router
}

func (impl *R) brokers(w http.ResponseWriter, r *http.Request, _ map[string]string) {
	brokers, err := persistence.AllBrokers(r.Context(), false)
	if err != nil {
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	data := make([]map[string]interface{}, 0)
	for _, b := range brokers {
		sum := sha256.Sum256([]byte("GET/assets"))
		token := jwt.NewWithClaims(jwt.SigningMethodRS512, jwt.MapClaims{
			"uid": b.BrokerId,
			"sid": b.SessionId,
			"scp": "ASSETS:READ",
			"exp": time.Now().Add(time.Hour * 24).Unix(),
			"sig": hex.EncodeToString(sum[:]),
		})

		block, _ := pem.Decode([]byte(b.SessionKey))
		privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}
		tokenString, err := token.SignedString(privateKey)
		if err != nil {
			render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}
		data = append(data, map[string]interface{}{
			"broker_id": b.BrokerId,
			"token":     tokenString,
		})
	}
	render.New().JSON(w, http.StatusOK, map[string]interface{}{"data": data})
}

func (impl *R) tokens(w http.ResponseWriter, r *http.Request, _ map[string]string) {
	var body struct {
		URI string `json:"uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		render.New().JSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}
	if !strings.HasPrefix(body.URI, "/network/snapshots") {
		render.New().JSON(w, http.StatusForbidden, map[string]interface{}{"error": "403"})
		return
	}

	sum := sha256.Sum256([]byte("GET" + body.URI))
	token := jwt.NewWithClaims(jwt.SigningMethodRS512, jwt.MapClaims{
		"uid": config.ClientId,
		"sid": config.SessionId,
		"scp": "SNAPSHOTS:READ",
		"exp": time.Now().Add(time.Hour * 24).Unix(),
		"sig": hex.EncodeToString(sum[:]),
	})

	block, _ := pem.Decode([]byte(config.SessionKey))
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	render.New().JSON(w, http.StatusOK, map[string]interface{}{"data": map[string]interface{}{"token": tokenString}})
}

func (impl *R) marketTicker(w http.ResponseWriter, r *http.Request, params map[string]string) {
	t, err := persistence.LastTrade(r.Context(), params["id"])
	if err != nil {
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	if t == nil {
		render.New().JSON(w, http.StatusOK, map[string]interface{}{"data": map[string]interface{}{}})
		return
	}
	b, err := cache.Book(r.Context(), params["id"], 1)
	if err != nil {
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	ticker := map[string]interface{}{
		"trade_id":  t.TradeId,
		"amount":    t.Amount,
		"price":     t.Price,
		"sequence":  b.Sequence,
		"timestamp": b.Timestamp,
		"ask":       "0",
		"bid":       "0",
	}
	data, _ := json.Marshal(b.Data)
	var best struct {
		Asks []struct {
			Price string `json:"price"`
		} `json:"asks"`
		Bids []struct {
			Price string `json:"price"`
		} `json:"bids"`
	}
	json.Unmarshal(data, &best)
	if len(best.Asks) > 0 {
		ticker["ask"] = best.Asks[0].Price
	}
	if len(best.Bids) > 0 {
		ticker["bid"] = best.Bids[0].Price
	}
	render.New().JSON(w, http.StatusOK, map[string]interface{}{"data": ticker})
}

func (impl *R) marketBook(w http.ResponseWriter, r *http.Request, params map[string]string) {
	book, err := cache.Book(r.Context(), params["id"], 0)
	if err != nil {
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
	} else {
		render.New().JSON(w, http.StatusOK, map[string]interface{}{"data": book})
	}
}

func (impl *R) marketTrades(w http.ResponseWriter, r *http.Request, params map[string]string) {
	order := r.URL.Query().Get("order")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := time.Parse(time.RFC3339Nano, r.URL.Query().Get("offset"))
	trades, err := persistence.MarketTrades(r.Context(), params["id"], offset, order, limit)
	if err != nil {
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	data := make([]map[string]interface{}, 0)
	for _, t := range trades {
		data = append(data, map[string]interface{}{
			"trade_id":   t.TradeId,
			"base":       t.BaseAssetId,
			"quote":      t.QuoteAssetId,
			"side":       t.Side,
			"price":      t.Price,
			"amount":     t.Amount,
			"created_at": t.CreatedAt,
		})
	}
	render.New().JSON(w, http.StatusOK, map[string]interface{}{"data": data})
}

func (impl *R) orders(w http.ResponseWriter, r *http.Request, params map[string]string) {
	userId, err := authenticateUser(r)
	if err != nil {
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	if userId == "" {
		render.New().JSON(w, http.StatusUnauthorized, map[string]interface{}{})
		return
	}

	market := r.URL.Query().Get("market")
	order := r.URL.Query().Get("order")
	state := r.URL.Query().Get("state")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := time.Parse(time.RFC3339Nano, r.URL.Query().Get("offset"))
	orders, err := persistence.UserOrders(r.Context(), userId, market, state, offset, order, limit)
	if err != nil {
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	data := make([]map[string]interface{}, 0)
	for _, o := range orders {
		data = append(data, map[string]interface{}{
			"order_id":         o.OrderId,
			"order_type":       o.OrderType,
			"base":             o.BaseAssetId,
			"quote":            o.QuoteAssetId,
			"side":             o.Side,
			"price":            o.Price,
			"remaining_amount": o.RemainingAmount,
			"filled_amount":    o.FilledAmount,
			"remaining_funds":  o.RemainingFunds,
			"filled_funds":     o.FilledFunds,
			"state":            o.State,
			"created_at":       o.CreatedAt,
		})
	}
	render.New().JSON(w, http.StatusOK, map[string]interface{}{"data": data})
}

func (impl *R) order(w http.ResponseWriter, r *http.Request, params map[string]string) {
	userId, err := authenticateUser(r)
	if err != nil {
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	if userId == "" {
		render.New().JSON(w, http.StatusUnauthorized, map[string]interface{}{})
		return
	}

	o, err := persistence.UserOrder(r.Context(), params["id"], userId)
	if err != nil {
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	if o == nil {
		render.New().JSON(w, http.StatusOK, map[string]interface{}{"data": map[string]interface{}{}})
		return
	}

	data := map[string]interface{}{
		"order_id":         o.OrderId,
		"order_type":       o.OrderType,
		"base":             o.BaseAssetId,
		"quote":            o.QuoteAssetId,
		"side":             o.Side,
		"price":            o.Price,
		"remaining_amount": o.RemainingAmount,
		"filled_amount":    o.FilledAmount,
		"remaining_funds":  o.RemainingFunds,
		"filled_funds":     o.FilledFunds,
		"state":            o.State,
		"created_at":       o.CreatedAt,
	}
	render.New().JSON(w, http.StatusOK, map[string]interface{}{"data": data})
}

func authenticateUser(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return "", nil
	}
	return persistence.Authenticate(r.Context(), header[7:])
}

func registerHanders(router *httptreemux.TreeMux) {
	router.MethodNotAllowedHandler = func(w http.ResponseWriter, r *http.Request, _ map[string]httptreemux.HandlerFunc) {
		render.New().JSON(w, http.StatusNotFound, map[string]interface{}{})
	}
	router.NotFoundHandler = func(w http.ResponseWriter, r *http.Request) {
		render.New().JSON(w, http.StatusNotFound, map[string]interface{}{})
	}
	router.PanicHandler = func(w http.ResponseWriter, r *http.Request, rcv interface{}) {
		err := fmt.Errorf(string(errors.New(rcv, 2).Stack()))
		render.New().JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
	}
}
