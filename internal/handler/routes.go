package handler

import (
	"crypto/rsa"
	
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// NewRouter builds the chi router with logging middleware and all metric
// routes registered against h. key enables request signing/verification
// when non-empty. privateKey, when non-nil, enables decrypting incoming
// request bodies encrypted via the X-Crypto-Key header. enablePprof mounts
// /debug/pprof profiler endpoints when true; it is off by default since
// profiles can leak data and the endpoints are a DoS vector.
func NewRouter(h *Handler, logger *zap.Logger, key string, enablePprof bool, privateKey *rsa.PrivateKey) *chi.Mux {
	mux := chi.NewMux()
	mux.Use(loggingMiddleware(logger))
	register(mux, h, logger, key, enablePprof, privateKey)
	return mux
}

func register(mux *chi.Mux, h *Handler, logger *zap.Logger, key string, enablePprof bool, privateKey *rsa.PrivateKey) {
	if enablePprof {
		mux.Mount("/debug", middleware.Profiler())
	}
	mux.Get("/ping", h.ping)
	
	mux.Group(
		func(mux chi.Router) {
			mux.Use(compressMiddleware(logger))
			mux.Get("/", h.getAll)
		},
	)
	
	mux.Group(
		func(mux chi.Router) {
			mux.Use(signMiddleware(key))
			mux.Use(compressMiddleware(logger))
			mux.Post("/update/{type}/{name}/{value}", h.update)
			mux.Get("/value/{type}/{name}", h.getValue)
		},
	)
	
	mux.Group(
		func(mux chi.Router) {
			mux.Use(signMiddleware(key))
			mux.Use(decryptMiddleware(privateKey, logger))
			mux.Use(compressMiddleware(logger))
			mux.Use(contentTypeMiddleware("application/json"))
			mux.Post("/update", h.updateJSON)
			mux.Post("/update/", h.updateJSON)
			mux.Post("/value", h.valueJSON)
			mux.Post("/value/", h.valueJSON)
			mux.Post("/updates", h.updatesJSON)
			mux.Post("/updates/", h.updatesJSON)
		},
	)
}
