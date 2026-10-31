package sitecode

import _ "embed"

// Types is anasaki.d.ts: the type definitions of the phase-1 script API
// (GET /api/v1/code/types, added to project downloads, Monaco extra lib).
//
//go:embed anasaki.d.ts
var Types string
