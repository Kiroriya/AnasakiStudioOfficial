// Package secretbox seals short secrets (API keys, passwords) with
// AES-256-GCM for storage in the database: "enc:v1:<base64(nonce|ciphertext)>".
// The key never lives in the database: SITE_CODE_SECRET_KEY (any string,
// hashed to 32 bytes) or a random key created once as site-code.key (0600)
// in PLUGINS_DIR. Site Code settings and plugin settings of type "secret"
// (CMS 2.1.0) use it.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// Prefix marks a sealed value.
	Prefix = "enc:v1:"
	// KeyFile is the key file in the key directory.
	KeyFile = "site-code.key"
)

// Box seals and opens values with a lazily loaded key.
type Box struct {
	envKey, dir string
	once        sync.Once
	key         []byte
	err         error
}

// New returns a box using envKey, else the key file in dir.
func New(envKey, dir string) *Box { return &Box{envKey: envKey, dir: dir} }

func (b *Box) loadKey() ([]byte, error) {
	b.once.Do(func() { b.key, b.err = LoadKey(b.envKey, b.dir) })
	return b.key, b.err
}

// Seal encrypts v ("" stays "").
func (b *Box) Seal(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	k, err := b.loadKey()
	if err != nil {
		return "", err
	}
	return Seal(k, v)
}

// Open decrypts a sealed value; any other value is returned as is.
func (b *Box) Open(v string) (string, error) {
	if !strings.HasPrefix(v, Prefix) {
		return v, nil
	}
	k, err := b.loadKey()
	if err != nil {
		return "", err
	}
	return Open(k, v)
}

// LoadKey returns the key: envKey hashed, else the key file in dir (created
// on first use; "./plugins-data" when dir is empty).
func LoadKey(envKey, dir string) ([]byte, error) {
	if v := strings.TrimSpace(envKey); v != "" {
		k := sha256.Sum256([]byte(v))
		return k[:], nil
	}
	if dir == "" {
		dir = "./plugins-data"
	}
	path := filepath.Join(dir, KeyFile)
	if b, err := os.ReadFile(path); err == nil {
		k, err := hex.DecodeString(strings.TrimSpace(string(b)))
		if err != nil || len(k) != 32 {
			return nil, fmt.Errorf("%s is not a valid key (64 hex characters)", path)
		}
		return k, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // two processes must not write different keys
	if errors.Is(err, os.ErrExist) {
		return LoadKey("", dir)
	}
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString(hex.EncodeToString(k) + "\n"); err != nil {
		f.Close()
		return nil, err
	}
	return k, f.Close()
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal encrypts v with key.
func Seal(key []byte, v string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return Prefix + base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(v), nil)), nil
}

// Open decrypts a sealed value with key (a value without the prefix is
// returned as is).
func Open(key []byte, v string) (string, error) {
	if !strings.HasPrefix(v, Prefix) {
		return v, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, Prefix))
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("sealed value too short")
	}
	b, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("cannot decrypt (SITE_CODE_SECRET_KEY or " + KeyFile + " changed?)")
	}
	return string(b), nil
}
