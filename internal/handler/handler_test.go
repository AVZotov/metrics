package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	models "github.com/AVZotov/metrics/internal/model"
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

func TestHandler_updateJSON(t *testing.T) {
	type want struct {
		statusCode  int
		contentType string
	}
	tests := []struct {
		name        string
		contentType string
		body        string
		want        want
	}{
		{
			name:        "counter valid",
			contentType: "application/json",
			body:        `{"id":"myCounter","type":"counter","delta":42}`,
			want:        want{statusCode: http.StatusOK, contentType: "application/json; charset=utf-8"},
		},
		{
			name:        "gauge valid",
			contentType: "application/json",
			body:        `{"id":"myGauge","type":"gauge","value":3.14}`,
			want:        want{statusCode: http.StatusOK, contentType: "application/json; charset=utf-8"},
		},
		{
			name:        "counter missing delta",
			contentType: "application/json",
			body:        `{"id":"myCounter","type":"counter"}`,
			want:        want{statusCode: http.StatusBadRequest},
		},
		{
			name:        "gauge missing value",
			contentType: "application/json",
			body:        `{"id":"myGauge","type":"gauge"}`,
			want:        want{statusCode: http.StatusBadRequest},
		},
		{
			name:        "unknown type",
			contentType: "application/json",
			body:        `{"id":"myMetric","type":"unknown","delta":1}`,
			want:        want{statusCode: http.StatusBadRequest},
		},
		{
			name:        "invalid json",
			contentType: "application/json",
			body:        `not json`,
			want:        want{statusCode: http.StatusBadRequest},
		},
		{
			name:        "wrong content type",
			contentType: "text/plain",
			body:        `{"id":"myCounter","type":"counter","delta":42}`,
			want:        want{statusCode: http.StatusUnsupportedMediaType},
		},
	}
	router := setupRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, tt.want.statusCode, w.Code)
			if tt.want.contentType != "" {
				assert.Equal(t, tt.want.contentType, w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestHandler_valueJSON(t *testing.T) {
	delta42 := int64(42)
	value314 := float64(3.14)
	type want struct {
		statusCode  int
		contentType string
		bodyDelta   *int64
		bodyValue   *float64
	}
	tests := []struct {
		name        string
		contentType string
		seedURL     string
		body        string
		want        want
	}{
		{
			name:        "counter existing",
			contentType: "application/json",
			seedURL:     "/update/counter/myCounter/42",
			body:        `{"id":"myCounter","type":"counter"}`,
			want:        want{statusCode: http.StatusOK, contentType: "application/json; charset=utf-8", bodyDelta: &delta42},
		},
		{
			name:        "gauge existing",
			contentType: "application/json",
			seedURL:     "/update/gauge/myGauge/3.14",
			body:        `{"id":"myGauge","type":"gauge"}`,
			want:        want{statusCode: http.StatusOK, contentType: "application/json; charset=utf-8", bodyValue: &value314},
		},
		{
			name:        "counter not found",
			contentType: "application/json",
			body:        `{"id":"unknown","type":"counter"}`,
			want:        want{statusCode: http.StatusNotFound},
		},
		{
			name:        "unknown type",
			contentType: "application/json",
			body:        `{"id":"myMetric","type":"unknown"}`,
			want:        want{statusCode: http.StatusBadRequest},
		},
		{
			name:        "empty id",
			contentType: "application/json",
			body:        `{"id":"","type":"counter"}`,
			want:        want{statusCode: http.StatusBadRequest},
		},
		{
			name:        "empty type",
			contentType: "application/json",
			body:        `{"id":"myCounter","type":""}`,
			want:        want{statusCode: http.StatusBadRequest},
		},
		{
			name:        "invalid json",
			contentType: "application/json",
			body:        `not json`,
			want:        want{statusCode: http.StatusBadRequest},
		},
		{
			name:        "wrong content type",
			contentType: "text/plain",
			body:        `{"id":"myCounter","type":"counter"}`,
			want:        want{statusCode: http.StatusUnsupportedMediaType},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupRouter()
			if tt.seedURL != "" {
				seedReq := httptest.NewRequest(http.MethodPost, tt.seedURL, nil)
				seedReq.Header.Set("Content-Type", "text/plain")
				router.ServeHTTP(httptest.NewRecorder(), seedReq)
			}
			req := httptest.NewRequest(http.MethodPost, "/value", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, tt.want.statusCode, w.Code)
			if tt.want.contentType != "" {
				assert.Equal(t, tt.want.contentType, w.Header().Get("Content-Type"))
			}
			if tt.want.bodyDelta != nil || tt.want.bodyValue != nil {
				var got models.Metrics
				assert.NoError(t, json.NewDecoder(w.Body).Decode(&got))
				if tt.want.bodyDelta != nil {
					assert.Equal(t, *tt.want.bodyDelta, *got.Delta)
				}
				if tt.want.bodyValue != nil {
					assert.Equal(t, *tt.want.bodyValue, *got.Value)
				}
			}
		})
	}
}

func TestContentTypeMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	tests := []struct {
		name        string
		contentType string
		wantStatus  int
	}{
		{
			name:        "exact match",
			contentType: "application/json",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "match with charset",
			contentType: "application/json; charset=utf-8",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "wrong type",
			contentType: "text/plain",
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "empty",
			contentType: "",
			wantStatus:  http.StatusUnsupportedMediaType,
		},
	}
	mw := ContentTypeMiddleware("application/json")(next)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()
			mw.ServeHTTP(w, req)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
