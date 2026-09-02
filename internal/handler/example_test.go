package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/AVZotov/metrics/internal/audit"
	"github.com/AVZotov/metrics/internal/config"
	models "github.com/AVZotov/metrics/internal/model"
	"github.com/AVZotov/metrics/internal/repository"
	"github.com/AVZotov/metrics/internal/service"
	"go.uber.org/zap"
)

// newExampleServer wires up a real in-memory Handler/Service/Repository
// chain, the same way handler_test.go does, and returns it as a running
// httptest.Server so the examples exercise the real HTTP stack.
func newExampleServer() *httptest.Server {
	store := repository.NewStore(repository.NewMemStore(), repository.NewNoopStore(), false)
	notifier := audit.NewNotifier(&config.AuditConfig{}, zap.NewNop())
	s := service.NewMetricsService(store, notifier)
	h := New(s, zap.NewNop())
	return httptest.NewServer(NewRouter(h, zap.NewNop(), "", false))
}

// ExampleNewRouter demonstrates saving a gauge metric via
// POST /update/{type}/{name}/{value}.
func ExampleNewRouter() {
	ts := newExampleServer()
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/update/gauge/Alloc/123.45", "text/plain", nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(resp.StatusCode)
	fmt.Printf("%q\n", string(body))
	// Output:
	// 200
	// ""
}

// ExampleNewRouter_updateJSON demonstrates saving a counter metric via
// POST /update with a JSON body.
func ExampleNewRouter_updateJSON() {
	ts := newExampleServer()
	defer ts.Close()

	delta := int64(10)
	metric := models.Metrics{ID: "PollCount", MType: models.Counter, Delta: &delta}
	body, _ := json.Marshal(metric)

	resp, err := http.Post(ts.URL+"/update", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Println(resp.StatusCode)
	fmt.Println(string(respBody))
	// Output:
	// 200
	// {"id":"PollCount","type":"counter","delta":10}
}

// ExampleNewRouter_getValue demonstrates a write-then-read cycle: save a
// gauge value with POST /update, then read it back with GET /value/{type}/{name}.
func ExampleNewRouter_getValue() {
	ts := newExampleServer()
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/update/gauge/HeapAlloc/42.5", "text/plain", nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/value/gauge/HeapAlloc")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
	// Output:
	// 42.5
}
