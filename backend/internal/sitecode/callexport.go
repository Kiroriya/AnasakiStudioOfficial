package sitecode

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/dop251/goja"
)

// ErrNoExport: the JS plugin is not running or does not declare the export.
var ErrNoExport = errors.New("export not available")

// HasExport reports whether the active JS plugin unit declares and registers
// export name (code.exports + site.export).
func (e *Engine) HasExport(unit, name string) bool {
	lp := e.units().get(unit)
	return lp != nil && lp.unit.exports[name] && lp.prog.exports[name]
}

// CallExport runs export name of the active JS plugin unit as a system call
// (no signed-in user, e.g. the legacy data migration of phase 7): arg is
// copied as JSON, the result comes back decoded from JSON. Errors are
// ErrNoExport or the unit's *RunError.
func (e *Engine) CallExport(unit, name string, arg any, timeout time.Duration) (any, error) {
	if !e.HasExport(unit, name) {
		return nil, ErrNoExport
	}
	argJSON, err := json.Marshal(arg)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	req, _ := http.NewRequest(http.MethodPost, "http://site.local/", nil)
	s := &Session{e: e, r: req, set: e.units(), setOK: true, lazy: true, system: true}
	defer s.Close()
	return s.callIn(unit, "export "+name, timeout, func(v *vm) goja.Callable { return v.exportFn(name) },
		func(v *vm) []goja.Value {
			parsed, err := v.parse(goja.Undefined(), v.rt.ToValue(string(argJSON)))
			if err != nil {
				return []goja.Value{goja.Null()}
			}
			return []goja.Value{parsed}
		})
}
