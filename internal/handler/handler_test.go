package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	
	"github.com/AVZotov/metrics/internal/repository"
	"github.com/AVZotov/metrics/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func setupRouter() chi.Router {
	r := repository.NewMemStorage()
	s := service.NewMetricsService(r)
	logger, _ := zap.NewDevelopment()
	h := New(s, logger)
	router := NewRouter(h, logger)
	return router
}

func TestHandler_update_Counter(t *testing.T) {
	type want struct {
		path        string
		contentType string
		statusCode  int
	}
	tests := []struct {
		name        string
		contentType string
		want        want
	}{
		{
			name:        "counter positive",
			contentType: "text/plain; charset=utf-8",
			want: want{
				path:        "http://localhost:8080/update/counter/testCounter/527",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusOK,
			},
		},
		{
			name:        "counter wrong metric type",
			contentType: "text/plain; charset=utf-8",
			want: want{
				path:        "http://localhost:8080/update/wrong/testCounter/527",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusBadRequest,
			},
		},
		{
			name:        "counter no metric name",
			contentType: "text/plain; charset=utf-8",
			want: want{
				path:        "http://localhost:8080/update/counter//527",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusNotFound,
			},
		},
		{
			name:        "counter not int value",
			contentType: "text/plain; charset=utf-8",
			want: want{
				path:        "http://localhost:8080/update/counter/testCounter/123.456",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusBadRequest,
			},
		},
		{
			name:        "gauge positive",
			contentType: "text/plain; charset=utf-8",
			want: want{
				path:        "http://localhost:8080/update/gauge/testCounter/123.456",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusOK,
			},
		},
		{
			name:        "gauge wrong Metric type",
			contentType: "text/plain; charset=utf-8",
			want: want{
				path:        "http://localhost:8080/update/wrong/testCounter/123.456",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusBadRequest,
			},
		},
		{
			name:        "gauge no metric name",
			contentType: "text/plain; charset=utf-8",
			want: want{
				path:        "http://localhost:8080/update/gauge//123.456",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusNotFound,
			},
		},
		{
			name:        "gauge not float value",
			contentType: "text/plain; charset=utf-8",
			want: want{
				path:        "http://localhost:8080/update/gauge/testCounter/abc",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusBadRequest,
			},
		},
	}
	router := setupRouter()
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodPost, tt.want.path, nil)
				request.Header.Add("Content-Type", tt.contentType)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, request)
				assert.Equal(t, tt.want.statusCode, w.Code)
				assert.Equal(t, tt.want.contentType, w.Header().Get("Content-Type"))
			},
		)
	}
}

func TestHandler_getValue(t *testing.T) {
	type want struct {
		statusCode  int
		contentType string
		body        string
	}
	tests := []struct {
		name        string
		contentType string
		seedURL     string
		getURL      string
		want        want
	}{
		{
			name:        "counter existing",
			contentType: "text/plain; charset=utf-8",
			seedURL:     "http://localhost:8080/update/counter/myCounter/42",
			getURL:      "http://localhost:8080/value/counter/myCounter",
			want: want{
				statusCode:  http.StatusOK,
				contentType: "text/plain; charset=utf-8",
				body:        "42",
			},
		},
		{
			name:        "gauge existing",
			contentType: "text/plain; charset=utf-8",
			seedURL:     "http://localhost:8080/update/gauge/myGauge/3.14",
			getURL:      "http://localhost:8080/value/gauge/myGauge",
			want: want{
				statusCode:  http.StatusOK,
				contentType: "text/plain; charset=utf-8",
				body:        "3.14",
			},
		},
		{
			name:        "counter not found",
			contentType: "text/plain; charset=utf-8",
			getURL:      "http://localhost:8080/value/counter/unknown",
			want: want{
				statusCode:  http.StatusNotFound,
				contentType: "text/plain; charset=utf-8",
			},
		},
		{
			name:        "gauge not found",
			contentType: "text/plain; charset=utf-8",
			getURL:      "http://localhost:8080/value/gauge/unknown",
			want: want{
				statusCode:  http.StatusNotFound,
				contentType: "text/plain; charset=utf-8",
			},
		},
		{
			name:        "unknown metric type",
			contentType: "text/plain; charset=utf-8",
			getURL:      "http://localhost:8080/value/wrong/myMetric",
			want: want{
				statusCode:  http.StatusBadRequest,
				contentType: "text/plain; charset=utf-8",
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				router := setupRouter()
				if tt.seedURL != "" {
					req := httptest.NewRequest(http.MethodPost, tt.seedURL, nil)
					req.Header.Add("Content-Type", tt.contentType)
					router.ServeHTTP(httptest.NewRecorder(), req)
				}
				req := httptest.NewRequest(http.MethodGet, tt.getURL, nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				assert.Equal(t, tt.want.statusCode, w.Code)
				assert.Equal(t, tt.want.contentType, w.Header().Get("Content-Type"))
				if tt.want.body != "" {
					assert.Equal(t, tt.want.body, w.Body.String())
				}
			},
		)
	}
}

func TestHandler_getAll(t *testing.T) {
	type want struct {
		statusCode  int
		contentType string
	}
	tests := []struct {
		name         string
		contentType  string
		seedURLs     []string
		bodyContains []string
		want         want
	}{
		{
			name:        "empty storage",
			contentType: "text/plain",
			want: want{
				statusCode:  http.StatusOK,
				contentType: "text/html; charset=utf-8",
			},
		},
		{
			name:        "with metrics",
			contentType: "text/plain",
			seedURLs: []string{
				"http://localhost:8080/update/counter/hits/10",
				"http://localhost:8080/update/gauge/temp/36.6",
			},
			bodyContains: []string{"hits", "temp"},
			want: want{
				statusCode:  http.StatusOK,
				contentType: "text/html; charset=utf-8",
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				router := setupRouter()
				for _, u := range tt.seedURLs {
					req := httptest.NewRequest(http.MethodPost, u, nil)
					req.Header.Add("Content-Type", tt.contentType)
					router.ServeHTTP(httptest.NewRecorder(), req)
				}
				req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				assert.Equal(t, tt.want.statusCode, w.Code)
				assert.Equal(t, tt.want.contentType, w.Header().Get("Content-Type"))
				for _, s := range tt.bodyContains {
					assert.True(t, strings.Contains(w.Body.String(), s), "body should contain %q", s)
				}
			},
		)
	}
}
