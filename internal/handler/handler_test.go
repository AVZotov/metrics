package handler

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	models "github.com/AVZotov/metrics/internal/model"
	"github.com/AVZotov/metrics/internal/repository"
	"github.com/AVZotov/metrics/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockService struct {
	updateFn func(mType, name, value string) error
	getFn    func(id, mType string) (*models.Metrics, error)
	getAllFn func() ([]*models.Metrics, error)
}

func (m *mockService) UpdateMetric(mType, name, value string) error {
	if m.updateFn != nil {
		return m.updateFn(mType, name, value)
	}
	return nil
}

func (m *mockService) GetMetric(id, mType string) (*models.Metrics, error) {
	if m.getFn != nil {
		return m.getFn(id, mType)
	}
	return nil, nil
}

func (m *mockService) GetMetrics() ([]*models.Metrics, error) {
	if m.getAllFn != nil {
		return m.getAllFn()
	}
	return nil, nil
}

func setupRouterWithService(svc service.Service) chi.Router {
	logger, _ := zap.NewDevelopment()
	h := New(svc, logger)
	return NewRouter(h, logger)
}

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

func TestHandler_updateJSON_ResponseBody(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantDelta *int64
		wantValue *float64
	}{
		{
			name:      "counter echoes delta in response",
			body:      `{"id":"hits","type":"counter","delta":42}`,
			wantDelta: func() *int64 { v := int64(42); return &v }(),
		},
		{
			name:      "gauge echoes value in response",
			body:      `{"id":"temp","type":"gauge","value":3.14}`,
			wantValue: func() *float64 { v := 3.14; return &v }(),
		},
	}
	router := setupRouter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			var got models.Metrics
			require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
			if tt.wantDelta != nil {
				require.NotNil(t, got.Delta)
				assert.Equal(t, *tt.wantDelta, *got.Delta)
			}
			if tt.wantValue != nil {
				require.NotNil(t, got.Value)
				assert.Equal(t, *tt.wantValue, *got.Value)
			}
		})
	}
}

func TestHandler_updateJSON_ServiceError_Returns500(t *testing.T) {
	sentinel := errors.New("storage failure")
	svc := &mockService{
		updateFn: func(_, _, _ string) error { return sentinel },
	}
	router := setupRouterWithService(svc)
	tests := []struct {
		name string
		body string
	}{
		{"counter service error", `{"id":"hits","type":"counter","delta":1}`},
		{"gauge service error", `{"id":"temp","type":"gauge","value":1.5}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusInternalServerError, w.Code)
		})
	}
}

func TestHandler_getValue_NilDelta_Returns500(t *testing.T) {
	svc := &mockService{
		getFn: func(id, _ string) (*models.Metrics, error) {
			return &models.Metrics{ID: id, MType: models.Counter, Delta: nil}, nil
		},
	}
	router := setupRouterWithService(svc)
	req := httptest.NewRequest(http.MethodGet, "/value/counter/myCounter", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_getValue_NilValue_Returns500(t *testing.T) {
	svc := &mockService{
		getFn: func(id, _ string) (*models.Metrics, error) {
			return &models.Metrics{ID: id, MType: models.Gauge, Value: nil}, nil
		},
	}
	router := setupRouterWithService(svc)
	req := httptest.NewRequest(http.MethodGet, "/value/gauge/myGauge", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_getAll_ServiceError_Returns500(t *testing.T) {
	svc := &mockService{
		getAllFn: func() ([]*models.Metrics, error) { return nil, errors.New("storage failure") },
	}
	router := setupRouterWithService(svc)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_valueJSON_ServiceInternalError_Returns500(t *testing.T) {
	svc := &mockService{
		getFn: func(_, _ string) (*models.Metrics, error) { return nil, errors.New("storage failure") },
	}
	router := setupRouterWithService(svc)
	req := httptest.NewRequest(http.MethodPost, "/value", strings.NewReader(`{"id":"x","type":"gauge"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestLoggingMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	})
	logger, _ := zap.NewDevelopment()
	mw := LoggingMiddleware(logger)(next)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "hello", w.Body.String())
}

func TestCompressMiddleware_Passthrough(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	mw := CompressMiddleware()(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "pong", w.Body.String())
	assert.Empty(t, w.Header().Get("Content-Encoding"))
}

func TestCompressMiddleware_InvalidGzipBody(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := CompressMiddleware()(next)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not gzip at all"))
	req.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCompressMiddleware_GzipRequestDecompression(t *testing.T) {
	payload := `{"id":"cpu","type":"gauge","value":1.5}`
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(payload))
	gz.Close()

	var gotBody string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	})
	mw := CompressMiddleware()(next)
	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, payload, gotBody)
}

func TestCompressMiddleware_GzipResponse_JSON(t *testing.T) {
	body := `{"id":"cpu","type":"gauge","value":1.5}`
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	mw := CompressMiddleware()(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	gr, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	defer gr.Close()
	decompressed, err := io.ReadAll(gr)
	require.NoError(t, err)
	assert.Equal(t, body, string(decompressed))
}

func TestCompressMiddleware_NoGzipForPlainText(t *testing.T) {
	body := "plain text response"
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(body))
	})
	mw := CompressMiddleware()(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, body, w.Body.String())
}

func TestCompressMiddleware_GzipResponse_HTML(t *testing.T) {
	body := "<html><body>metrics</body></html>"
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	})
	mw := CompressMiddleware()(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	gr, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	defer gr.Close()
	decompressed, err := io.ReadAll(gr)
	require.NoError(t, err)
	assert.Equal(t, body, string(decompressed))
}

func TestCompressMiddleware_MultiWrite(t *testing.T) {
	const expected = "pingpong"
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("ping"))
		_, _ = w.Write([]byte("pong"))
	})
	mw := CompressMiddleware()(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)
	gr, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	defer gr.Close()
	decompressed, err := io.ReadAll(gr)
	require.NoError(t, err)
	assert.Equal(t, expected, string(decompressed))
}

// Test to check Accept-Encoding.
// Agent not set req.Header.Set("Accept-Encoding", "gzip")
// http.Client set this header itself
func TestCompressMiddleware_GzipBothDirections(t *testing.T) {
	reqBody := `{"id":"cpu","type":"gauge","value":1.5}`
	respBody := `{"id":"cpu","type":"gauge","value":1.5}`

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(reqBody))
	gz.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		require.Equal(t, reqBody, string(b))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respBody))
	})
	mw := CompressMiddleware()(next)
	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)

	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	gr, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	defer gr.Close()
	decompressed, err := io.ReadAll(gr)
	require.NoError(t, err)
	assert.Equal(t, respBody, string(decompressed))
}
