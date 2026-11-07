package plugin

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// Package signatures (2.4.0, store 1.5.0, security assessment S2). The store
// signs "anasaki-plugin\n<id>\n<version>\n<sha256>" with Ed25519 and sends it
// as X-Anasaki-Signature: ed25519:<keyId>:<base64>. The key a site trusts:
//   - STORE_PUBLIC_KEY (base64), when set;
//   - else OfficialStoreKey for the official store;
//   - else the key another store showed the first time it signed (kept in
//     the options, "trust on first use").
// With a trusted key, a package without a valid signature is never installed;
// a store that has never signed is accepted as before (older stores).

// OfficialStoreKey is the public key of the official Plugin Store (OfficialStoreURL).
const OfficialStoreKey = "nPfH52OKGpwxbNwkfSwzFlEaUNrESTiCSiUAaeKesS4="

// optionStoreKeyPrefix + a hash of the store URL holds a key learned on first use.
const optionStoreKeyPrefix = "store_signing_key:"

func storeKeyOption(base string) string {
	s := sha256.Sum256([]byte(strings.ToLower(base)))
	return optionStoreKeyPrefix + hex.EncodeToString(s[:8])
}

func decodeKey(b64 string) ed25519.PublicKey {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil || len(k) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(k)
}

// trustedStoreKey is the key the current store must sign with (nil = none yet).
func (m *Manager) trustedStoreKey() ed25519.PublicKey {
	if v := strings.TrimSpace(os.Getenv("STORE_PUBLIC_KEY")); v != "" {
		return decodeKey(v)
	}
	base := m.StoreURL()
	if strings.EqualFold(base, OfficialStoreURL) {
		return decodeKey(OfficialStoreKey)
	}
	if m.host != nil && m.host.Store != nil {
		if opts, err := m.host.Store.Options(); err == nil {
			return decodeKey(opts[storeKeyOption(base)])
		}
	}
	return nil
}

// learnStoreKey asks a store that signs for its public key and keeps it.
func (m *Manager) learnStoreKey() ed25519.PublicKey {
	b, _, err := m.storeDo(http.MethodGet, "/api/v1/signing-key", nil, nil, http.Header{"Accept": {"application/json"}}, 64<<10)
	if err != nil {
		return nil
	}
	var v struct{ Alg, PublicKey string }
	if json.Unmarshal(b, &v) != nil || !strings.EqualFold(v.Alg, "ed25519") {
		return nil
	}
	k := decodeKey(v.PublicKey)
	if k != nil && m.host != nil && m.host.Store != nil {
		if err := m.host.Store.SetOptions(map[string]string{storeKeyOption(m.StoreURL()): v.PublicKey}); err != nil {
			log.Printf("plugin store: keep signing key: %v", err)
		}
	}
	return k
}

// verifyPackage checks a store download: its signature against the trusted
// key, and that the version it was signed for is the one inside the zip.
func (m *Manager) verifyPackage(id string, b []byte, hdr http.Header) error {
	sigHdr := strings.TrimSpace(hdr.Get("X-Anasaki-Signature"))
	key := m.trustedStoreKey()
	if sigHdr == "" {
		if key != nil {
			return pkgErr("the store sent the package without a signature — it was not installed")
		}
		return nil // a store that does not sign
	}
	if key == nil {
		if key = m.learnStoreKey(); key == nil {
			return pkgErr("the store signed the package but its public key could not be read — try again")
		}
	}
	parts := strings.SplitN(sigHdr, ":", 3)
	sig, err := base64.StdEncoding.DecodeString(parts[len(parts)-1])
	if len(parts) != 3 || parts[0] != "ed25519" || err != nil {
		return pkgErr("the package signature is malformed — it was not installed")
	}
	version := strings.TrimSpace(hdr.Get("X-Anasaki-Version"))
	if mv := zipManifestVersion(b); version == "" || (mv != "" && mv != version) {
		return pkgErr("the package does not match the version the store signed — it was not installed")
	}
	sum := sha256.Sum256(b)
	if !ed25519.Verify(key, []byte("anasaki-plugin\n"+id+"\n"+version+"\n"+hex.EncodeToString(sum[:])), sig) {
		return pkgErr("the package signature is not valid for this store — it was not installed")
	}
	return nil
}

// zipManifestVersion is the version in a package's manifest.json ("" when unreadable).
func zipManifestVersion(b []byte) string {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return ""
	}
	for _, f := range zr.File {
		if f.Name != "manifest.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return ""
		}
		defer rc.Close()
		raw, _ := io.ReadAll(io.LimitReader(rc, 1<<20))
		var mf struct{ Version string }
		_ = json.Unmarshal(raw, &mf)
		return mf.Version
	}
	return ""
}
