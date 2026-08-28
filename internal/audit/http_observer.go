package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var _ Observer = (*httpObserver)(nil)

const requestTimeout = 5 * time.Second

type httpObserver struct {
	client   *http.Client
	auditURL string
}

func newHTTPObserver(auditURL string) *httpObserver {
	c := new(http.Client)
	c.Timeout = requestTimeout

	return &httpObserver{
		client:   c,
		auditURL: auditURL,
	}
}

func (h *httpObserver) Notify(event Event) error {
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(event)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, h.auditURL, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("audit server returned status %d", resp.StatusCode)
	}

	return nil
}

func (h *httpObserver) Name() string {
	return h.auditURL
}
