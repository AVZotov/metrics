package audit

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPObserver_Notify_SendsPOSTWithEvent(t *testing.T) {
	var (
		gotMethod      string
		gotContentType string
		gotBody        Event
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	obs := newHTTPObserver(server.URL)
	event := NewEvent([]string{"cpu"}, "1.2.3.4")

	err := obs.Notify(event)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, event, gotBody)
}

func TestHTTPObserver_Notify_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	obs := newHTTPObserver(server.URL)
	err := obs.Notify(NewEvent([]string{"cpu"}, "1.2.3.4"))
	assert.NoError(t, err)
}

func TestHTTPObserver_Notify_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	obs := newHTTPObserver(server.URL)
	err := obs.Notify(NewEvent([]string{"cpu"}, "1.2.3.4"))
	assert.Error(t, err)
}

func TestHTTPObserver_Notify_UnreachableServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	obs := newHTTPObserver(url)
	err := obs.Notify(NewEvent([]string{"cpu"}, "1.2.3.4"))
	assert.Error(t, err)
}
