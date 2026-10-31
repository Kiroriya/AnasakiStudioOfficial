//go:build !wasip1

package anasaki

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const inWasm = false

func newClient(cfg Config) *Client {
	c := &Client{id: cfg.ID, secret: cfg.Secret, base: strings.TrimRight(cfg.CMSURL, "/"),
		http: &http.Client{Timeout: 20 * time.Second}}
	c.transport = c.httpTransport
	return c
}

func run(p *Plugin, addr string) error {
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}
	log.Printf("%s listening on %s", p.cfg.Name, addr)
	return http.ListenAndServe(addr, p)
}
