package controllers

import (
	"net/http"
	"runtime"

	"github.com/dimfeld/httptreemux"
	"github.com/usmbest/ocean.one/example/config"
	"github.com/usmbest/ocean.one/example/views"
)

func RegisterRoutes(router *httptreemux.TreeMux) {
	router.GET("/", root)
	router.GET("/_hc", healthCheck)

	registerVerifications(router)
	registerUsers(router)
	registerSessions(router)
	registerTokens(router)
	registerOrders(router)
	registerWithdrawals(router)
	registerMarkets(router)
}

func root(w http.ResponseWriter, r *http.Request, params map[string]string) {
	views.RenderDataResponse(w, r, map[string]string{
		"build":      config.BuildVersion + "-" + runtime.Version(),
		"developers": "https://github.com/usmbest/ocean.one/example",
	})
}

func healthCheck(w http.ResponseWriter, r *http.Request, params map[string]string) {
	views.RenderBlankResponse(w, r)
}
