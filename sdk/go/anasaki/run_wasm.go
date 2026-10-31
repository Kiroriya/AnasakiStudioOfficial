//go:build wasip1

package anasaki

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"unsafe"
)

const inWasm = true

//go:wasmimport anasaki call
func hostCall(ptr, size uint32) uint32

//go:wasmimport anasaki result
func hostResult(ptr uint32)

func newClient(cfg Config) *Client {
	c := &Client{id: cfg.ID}
	c.transport = wasmTransport
	return c
}

// wasmTransport asks the CMS through the host functions.
func wasmTransport(_ context.Context, method, uri string, body []byte) (int, []byte, error) {
	req, _ := json.Marshal(map[string]any{"method": method, "uri": uri, "body": json.RawMessage(nonEmpty(body))})
	n := hostCall(uint32(uintptr(unsafe.Pointer(&req[0]))), uint32(len(req)))
	if n == 0 {
		return 0, nil, io.ErrUnexpectedEOF
	}
	buf := make([]byte, n)
	hostResult(uint32(uintptr(unsafe.Pointer(&buf[0]))))
	var out struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(buf, &out); err != nil {
		return 0, nil, err
	}
	return out.Status, out.Body, nil
}

func nonEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte("null")
	}
	return b
}

type envelope struct {
	Method string      `json:"method,omitempty"`
	URI    string      `json:"uri,omitempty"`
	Status int         `json:"status,omitempty"`
	Header http.Header `json:"header"`
	Body   []byte      `json:"body"`
}

// run answers the single request on stdin and writes the response to stdout.
func run(p *Plugin, _ string) error {
	p.trusted = true
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	var in envelope
	if err := json.Unmarshal(raw, &in); err != nil {
		return err
	}
	req := httptest.NewRequest(in.Method, in.URI, bytes.NewReader(in.Body))
	for k, v := range in.Header {
		req.Header[k] = v
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)
	return json.NewEncoder(os.Stdout).Encode(envelope{Status: w.Code, Header: w.Header(), Body: w.Body.Bytes()})
}
