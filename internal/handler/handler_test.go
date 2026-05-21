package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AVZotov/metrics/internal/repository"
	"github.com/AVZotov/metrics/internal/service"
	"github.com/stretchr/testify/assert"
)

func setupHandler() *Handler {
	r := repository.NewMemStorage()
	s := service.NewMetricsService(r)
	return New(s)
}

func TestHandler_update_Counter(t *testing.T) {
	type want struct {
		path        string
		contentType string
		statusCode  int
	}
	tests := []struct {
		name string
		want want
	}{
		{
			name: "counter positive",
			want: want{
				path:        "http://localhost:8080/update/counter/testCounter/527",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusOK,
			},
		},
		{
			name: "counter wrong metric type",
			want: want{
				path:        "http://localhost:8080/update/wrong/testCounter/527",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusBadRequest,
			},
		},
		{
			name: "counter no metric name",
			want: want{
				path:        "http://localhost:8080/update/counter//527",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusNotFound,
			},
		},
		{
			name: "counter not int value",
			want: want{
				path:        "http://localhost:8080/update/counter/testCounter/123.456",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusBadRequest,
			},
		},
		{
			name: "gauge positive",
			want: want{
				path:        "http://localhost:8080/update/gauge/testCounter/123.456",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusOK,
			},
		},
		{
			name: "gauge wrong Metric type",
			want: want{
				path:        "http://localhost:8080/update/wrong/testCounter/123.456",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusBadRequest,
			},
		},
		{
			name: "gauge no metric name",
			want: want{
				path:        "http://localhost:8080/update/gauge//123.456",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusNotFound,
			},
		},
		{
			name: "gauge not float value",
			want: want{
				path:        "http://localhost:8080/update/gauge/testCounter/abc",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusBadRequest,
			},
		},
	}
	h := setupHandler()
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodPost, tt.want.path, nil)
				w := httptest.NewRecorder()
				router := NewRouter(h)
				router.ServeHTTP(w, request)
				assert.Equal(t, tt.want.statusCode, w.Code)
				assert.Equal(t, tt.want.contentType, w.Header().Get("Content-Type"))
			},
		)
	}
}

func TestHandler_badRequest(t *testing.T) {
	type want struct {
		path        string
		contentType string
		statusCode  int
	}
	tests := []struct {
		name string
		want want
	}{
		{
			name: "positive",
			want: want{
				path:        "http://localhost:8080/update",
				contentType: "text/plain; charset=utf-8",
				statusCode:  http.StatusBadRequest,
			},
		},
	}
	h := setupHandler()
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodPost, tt.want.path, nil)
				w := httptest.NewRecorder()
				router := NewRouter(h)
				router.ServeHTTP(w, request)
				assert.Equal(t, tt.want.statusCode, w.Code)
				assert.Equal(t, tt.want.contentType, w.Header().Get("Content-Type"))
			},
		)
	}
}
