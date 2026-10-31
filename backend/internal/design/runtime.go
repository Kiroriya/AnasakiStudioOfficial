package design

import (
	_ "embed"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
)

//go:embed runtime.js
var runtimeSource string

var (
	runtimeOnce sync.Once
	runtimeMin  string
)

// RuntimeScript is the Site Code client runtime (minified once, on first
// use). Pages include it only when Output.Runtime is set, after
// `window.__reaP=<postId>`.
func RuntimeScript() string {
	runtimeOnce.Do(func() {
		res := api.Transform(runtimeSource, api.TransformOptions{Loader: api.LoaderJS, Target: api.ES2017,
			MinifyWhitespace: true, MinifyIdentifiers: true, MinifySyntax: true, LegalComments: api.LegalCommentsNone, Charset: api.CharsetUTF8})
		if len(res.Errors) == 0 {
			runtimeMin = string(res.Code)
		} else {
			runtimeMin = runtimeSource
		}
	})
	return runtimeMin
}
