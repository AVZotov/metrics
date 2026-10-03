package handler

import (
	"crypto/rsa"
	"net/http"
	"net/netip"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// RouterOptions holds optional router features. The zero value of every
// field disables its feature, so callers set only what they need.
type RouterOptions struct {
	// Key enables request signing/verification when non-empty.
	Key string
	// EnablePprof mounts /debug/pprof profiler endpoints when true. It is
	// off by default since profiles can leak data and the endpoints are a
	// DoS vector.
	EnablePprof bool
	// PrivateKey, when non-nil, enables decrypting incoming request bodies
	// encrypted via the X-Crypto-Key header.
	PrivateKey *rsa.PrivateKey
	// TrustedSubnet, when valid, restricts metric write endpoints to agents
	// whose X-Real-IP belongs to it. The zero value disables the check.
	TrustedSubnet netip.Prefix
}

// NewRouter builds the chi router with logging middleware and all metric
// routes registered against h.
func NewRouter(h *Handler, logger *zap.Logger, opts RouterOptions) *chi.Mux {
	mux := chi.NewMux()
	mux.Use(loggingMiddleware(logger))
	register(mux, h, logger, opts)
	return mux
}

func register(mux *chi.Mux, h *Handler, logger *zap.Logger, opts RouterOptions) {
	// Each call builds fresh middleware instances, so every group gets its
	// own chain, the same as listing the middleware inline per group.

	// Text-format API: /update/{type}/{name}/{value}, /value/{type}/{name}.
	plain := func() chi.Middlewares {
		return chi.Chain(
			signMiddleware(opts.Key),
			compressMiddleware(logger),
		)
	}

	// JSON API. Middleware order is required, not cosmetic: it must undo
	// the agent's encoding in reverse. The agent sends
	// sign(encrypt(gzip(json))), so the signature is checked over the raw
	// encrypted bytes first, then the body is decrypted, and only then
	// decompressed. Reordering these doesn't fail loudly: the requests
	// just start getting 400s from whichever middleware receives bytes it
	// can't read.
	jsonAPI := func() chi.Middlewares {
		return chi.Chain(
			signMiddleware(opts.Key),
			decryptMiddleware(opts.PrivateKey, logger),
			compressMiddleware(logger),
			contentTypeMiddleware("application/json"),
		)
	}

	registerPublic(mux, h, logger, opts.EnablePprof)
	registerReads(mux, h, plain, jsonAPI)
	registerWrites(mux, h, trustedSubnetMiddleware(opts.TrustedSubnet, logger), plain, jsonAPI)
}

// registerPublic mounts endpoints that need no signature: the health
// check, the HTML dashboard and, when enabled, the pprof profiler.
func registerPublic(r chi.Router, h *Handler, logger *zap.Logger, enablePprof bool) {
	if enablePprof {
		r.Mount("/debug", middleware.Profiler())
	}
	r.Get("/ping", h.ping)
	r.With(compressMiddleware(logger)).Get("/", h.getAll)
}

// registerReads mounts metric read endpoints. They are not restricted by
// source IP: any client that passes the signature check may read.
func registerReads(r chi.Router, h *Handler, plain, jsonAPI func() chi.Middlewares) {
	r.With(plain()...).Get("/value/{type}/{name}", h.getValue)

	r.Group(func(r chi.Router) {
		r.Use(jsonAPI()...)
		r.Post("/value", h.valueJSON)
		r.Post("/value/", h.valueJSON)
	})
}

// registerWrites mounts metric write endpoints. The trusted subnet check
// runs first: it only reads a header, so untrusted agents get 403 before
// any body is read for signature verification or decryption.
func registerWrites(
	r chi.Router, h *Handler, trusted func(http.Handler) http.Handler,
	plain, jsonAPI func() chi.Middlewares,
) {
	r.Group(func(r chi.Router) {
		r.Use(trusted)

		r.Group(func(r chi.Router) {
			r.Use(plain()...)
			r.Post("/update/{type}/{name}/{value}", h.update)
		})

		r.Group(func(r chi.Router) {
			r.Use(jsonAPI()...)
			r.Post("/update", h.updateJSON)
			r.Post("/update/", h.updateJSON)
			r.Post("/updates", h.updatesJSON)
			r.Post("/updates/", h.updatesJSON)
		})
	})
}
