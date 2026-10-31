package anasaki

// Run serves the plugin. Built normally it listens on addr (or $ADDR) as a
// standalone service; built with GOOS=wasip1 GOARCH=wasm it answers the one
// request the CMS hands it on stdin — the same code works both ways.
func Run(p *Plugin, addr string) error { return run(p, addr) }
