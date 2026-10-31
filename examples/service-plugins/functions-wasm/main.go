// functions-wasm is a minimal WASM service plugin without the SDK: it
// answers plugins.call('reports', 'monthly', input) from JS code and logs the
// events it subscribed to. Build:
//
//	GOOS=wasip1 GOARCH=wasm go build -o package/plugin.wasm .
//
// The CMS runs the module once per request: the request envelope
// {method, uri, header, body(base64)} on stdin, the response envelope
// {status, header, body(base64)} on stdout.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type envelope struct {
	Method string              `json:"method,omitempty"`
	URI    string              `json:"uri,omitempty"`
	Status int                 `json:"status,omitempty"`
	Header map[string][]string `json:"header"`
	Body   []byte              `json:"body"`
}

func reply(status int, v any) {
	b, _ := json.Marshal(v)
	out, _ := json.Marshal(envelope{Status: status, Header: map[string][]string{"Content-Type": {"application/json"}}, Body: b})
	os.Stdout.Write(out)
}

func main() {
	raw, _ := io.ReadAll(os.Stdin)
	var in envelope
	if err := json.Unmarshal(raw, &in); err != nil {
		reply(400, map[string]string{"error": "bad envelope"})
		return
	}
	switch {
	case in.URI == "/health":
		reply(200, map[string]string{"name": "Reports", "version": "1.0.0"})
	case strings.HasPrefix(in.URI, "/functions/monthly"):
		var call struct {
			Input struct {
				Sales []float64 `json:"sales"`
			} `json:"input"`
			User    any `json:"user"`
			Visitor any `json:"visitor"`
		}
		_ = json.Unmarshal(in.Body, &call)
		total := 0.0
		for _, s := range call.Input.Sales {
			total += s
		}
		avg := 0.0
		if n := len(call.Input.Sales); n > 0 {
			avg = total / float64(n)
		}
		reply(200, map[string]any{"result": map[string]any{"total": total, "average": avg, "count": len(call.Input.Sales),
			"signedIn": call.User != nil}})
	case strings.HasPrefix(in.URI, "/functions/"):
		reply(404, map[string]string{"error": "no such function"})
	case in.URI == "/events":
		fmt.Fprintf(os.Stderr, "event: %s\n", in.Body)
		reply(200, map[string]bool{"ok": true})
	default:
		reply(404, map[string]string{"error": "not found"})
	}
}
