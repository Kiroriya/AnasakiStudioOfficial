package plugin

import (
	"encoding/json"
	"log"
	"slices"
)

// Settings of type "secret" (CMS 2.1.0): sealed in plugin_packages.settings
// (secretbox), opened when the package list loads, so site.settings and the
// service API see the value; the admin view and public templates get "".

// sealedJSON is v as stored: secret values sealed.
func (p *packages) sealedJSON(schema []Field, v Values) ([]byte, error) {
	out := Values{}
	for k, x := range v {
		out[k] = x
	}
	for _, f := range schema {
		s, ok := out[f.Key].(string)
		if f.Type != FieldSecret || !ok || s == "" {
			continue
		}
		sealed, err := p.box.Seal(s)
		if err != nil {
			return nil, err
		}
		out[f.Key] = sealed
	}
	return json.Marshal(out)
}

// openSecrets opens the sealed values of stored (a value that cannot be
// opened, e.g. after the key changed, becomes "" and is logged).
func (p *packages) openSecrets(id string, schema []Field, stored map[string]any) map[string]any {
	for _, f := range schema {
		s, ok := stored[f.Key].(string)
		if f.Type != FieldSecret || !ok || s == "" {
			continue
		}
		v, err := p.box.Open(s)
		if err != nil {
			log.Printf("plugin %s: setting %s: %v", id, f.Key, err)
			v = ""
		}
		stored[f.Key] = v
	}
	return stored
}

// maskSecrets is v with every secret value "" (admin views, public templates).
func maskSecrets(schema []Field, v Values) Values {
	if !slices.ContainsFunc(schema, func(f Field) bool { return f.Type == FieldSecret }) {
		return v
	}
	out := Values{}
	for k, x := range v {
		out[k] = x
	}
	for _, f := range schema {
		if f.Type == FieldSecret {
			out[f.Key] = ""
		}
	}
	return out
}

// secretsSet lists the secret settings that have a value.
func secretsSet(schema []Field, v Values) []string {
	var out []string
	for _, f := range schema {
		if s, _ := v[f.Key].(string); f.Type == FieldSecret && s != "" {
			out = append(out, f.Key)
		}
	}
	return out
}

// keepSecrets fills empty or missing secrets of in from cur (the admin form
// sends "" for "unchanged"). Clearing a secret: the key "<key>__clear": true.
func keepSecrets(schema []Field, in map[string]any, cur Values) map[string]any {
	out := map[string]any{}
	for k, x := range in {
		out[k] = x
	}
	for _, f := range schema {
		if f.Type != FieldSecret {
			continue
		}
		clear, _ := out[f.Key+"__clear"].(bool)
		delete(out, f.Key+"__clear")
		if s, _ := out[f.Key].(string); s == "" && !clear {
			out[f.Key] = cur[f.Key]
		}
	}
	return out
}
