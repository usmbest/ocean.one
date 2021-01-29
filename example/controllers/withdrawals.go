package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/dimfeld/httptreemux"
	"github.com/usmbest/go-number"
	"github.com/usmbest/ocean.one/example/middlewares"
	"github.com/usmbest/ocean.one/example/session"
	"github.com/usmbest/ocean.one/example/views"
)

type withdrawalsImpl struct{}

type withdrawalRequest struct {
	TraceId string `json:"trace_id"`
	AssetId string `json:"asset_id"`
	Amount  string `json:"amount"`
	Memo    string `json:"memo"`
}

func registerWithdrawals(router *httptreemux.TreeMux) {
	impl := &withdrawalsImpl{}

	router.POST("/withdrawals", impl.create)
}

func (impl *withdrawalsImpl) create(w http.ResponseWriter, r *http.Request, _ map[string]string) {
	var body withdrawalRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		views.RenderErrorResponse(w, r, session.BadRequestError(r.Context()))
		return
	}

	err := middlewares.CurrentUser(r).CreateWithdrawal(r.Context(), body.AssetId, number.FromString(body.Amount), body.TraceId, body.Memo)
	if err != nil {
		views.RenderErrorResponse(w, r, err)
	} else {
		views.RenderBlankResponse(w, r)
	}
}
