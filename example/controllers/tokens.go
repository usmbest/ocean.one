package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/dimfeld/httptreemux"
	"github.com/usmbest/ocean.one/example/middlewares"
	"github.com/usmbest/ocean.one/example/session"
	"github.com/usmbest/ocean.one/example/views"
)

type tokensImpl struct{}

type tokenRequest struct {
	Category string `json:"category"`
	URI      string `json:"uri"`
}

func registerTokens(router *httptreemux.TreeMux) {
	impl := &tokensImpl{}

	router.POST("/tokens", impl.create)
}

func (impl *tokensImpl) create(w http.ResponseWriter, r *http.Request, _ map[string]string) {
	var body tokenRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		views.RenderErrorResponse(w, r, session.BadRequestError(r.Context()))
		return
	}

	key := middlewares.CurrentUser(r).Key
	switch body.Category {
	case "MIXIN":
		token, err := key.MixinToken(r.Context(), body.URI)
		if err != nil {
			views.RenderErrorResponse(w, r, err)
		} else {
			views.RenderDataResponse(w, r, map[string]string{"token": token})
		}
	case "OCEAN":
		token, err := key.OceanToken(r.Context())
		if err != nil {
			views.RenderErrorResponse(w, r, err)
		} else {
			views.RenderDataResponse(w, r, map[string]string{"token": token})
		}
	default:
		views.RenderErrorResponse(w, r, session.BadDataError(r.Context()))
	}
}
