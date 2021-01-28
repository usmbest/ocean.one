package middlewares

import (
	"net/http"
	"strings"

	"github.com/usmbest/ocean.one/example/durable"
	"github.com/usmbest/ocean.one/example/session"
	"github.com/usmbest/ocean.one/example/uuid"
)

func Log(handler http.Handler, client *durable.LoggerClient, service string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.ToUpper(uuid.NewV4().String())
		r.Header["X-Request-Id"] = []string{id}
		logger := durable.BuildLogger(client, service, r)
		ctx := session.WithLogger(r.Context(), logger)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}
