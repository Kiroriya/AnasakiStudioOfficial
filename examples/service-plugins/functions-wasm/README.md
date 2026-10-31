# Reports — WASM functions example

A service plugin compiled to WebAssembly, written without the SDK, that exposes a function to JS code:

```sh
GOOS=wasip1 GOARCH=wasm go build -o package/plugin.wasm .
cd package && zip -r ../reports-1.0.0.zip .     # manifest.json + plugin.wasm
```

```ts
// Site Code (or a JS plugin that lists "reports" in requires)
const r = plugins.call('reports', 'monthly', { sales: [100, 250] });   // { total: 350, average: 175, count: 2, signedIn }
```

The CMS POSTs `{input, user, visitor}` to `/functions/monthly`; the answer `{"result": …}` is what
`plugins.call` returns (`{"error": "…"}` with a 4xx/5xx status throws). It also receives the
`shop.order.paid` event emitted by the shop JS plugin (`site.emit`).
