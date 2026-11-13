// Package update finds a newer release of the program and fetches it safely. A release is
// described by a small JSON manifest; the installer it points at is accepted only when its
// size and SHA-256 match and the manifest carries a valid Ed25519 signature made with the
// publisher's key, so that neither the server nor the network can substitute another file.
package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Manifest describes the newest release.
type Manifest struct {
	Version   string `json:"version"`
	URL       string `json:"url"`       // the installer
	Size      int64  `json:"size"`      // its size in bytes
	SHA256    string `json:"sha256"`    // of the installer, hex
	Signature string `json:"signature"` // base64 Ed25519 signature of SignedText(Version, SHA256)
	Notes     string `json:"notes"`     // what is new, plain text
}

const (
	maxManifest  = 64 << 10
	maxInstaller = 200 << 20
)

// SignedText is what the publisher signs: the version and the hash of the installer, so a
// signature made for one release cannot be moved to another file or version.
func SignedText(version, sha256hex string) []byte {
	return []byte("linnet-update-v1\n" + version + "\n" + strings.ToLower(sha256hex) + "\n")
}

// Newer reports whether candidate is a later version than current. Versions are numbers
// separated by dots; anything after the numbers of a part is ignored.
func Newer(candidate, current string) bool {
	a, b := parts(candidate), parts(current)
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func parts(v string) []int {
	var out []int
	for _, p := range strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".") {
		digits := p
		for i, r := range p {
			if r < '0' || r > '9' {
				digits = p[:i]
				break
			}
		}
		n, _ := strconv.Atoi(digits)
		out = append(out, n)
	}
	return out
}

// Source is where releases are published and whose signature they must carry.
type Source struct {
	ManifestURL string
	PublicKey   ed25519.PublicKey
	// AllowLoopback lets plain http to this computer through, for tests only.
	AllowLoopback bool
	Client        *http.Client
}

func (s Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return errors.New("too many redirects")
			}
			return s.allowed(req.URL)
		},
	}
}

// allowed accepts https anywhere and, for tests, http to this computer.
func (s Source) allowed(u *url.URL) error {
	if u.Scheme == "https" && u.Host != "" {
		return nil
	}
	if s.AllowLoopback && u.Scheme == "http" {
		if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return fmt.Errorf("address not allowed: %s://%s", u.Scheme, u.Host)
}

func (s Source) get(ctx context.Context, raw string) (*http.Response, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if err := s.allowed(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Linnet-Updater")
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("server answered %d", resp.StatusCode)
	}
	return resp, nil
}

// Check reads the manifest and verifies its signature. The manifest it returns can be
// trusted to come from the publisher; whether it is newer is for the caller to decide.
func (s Source) Check(ctx context.Context) (Manifest, error) {
	var m Manifest
	if len(s.PublicKey) != ed25519.PublicKeySize {
		return m, errors.New("no publisher key")
	}
	resp, err := s.get(ctx, s.ManifestURL)
	if err != nil {
		return m, fmt.Errorf("read manifest: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxManifest+1))
	if err != nil {
		return m, fmt.Errorf("read manifest: %w", err)
	}
	if len(raw) > maxManifest {
		return m, errors.New("manifest too large")
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("manifest is not valid: %w", err)
	}
	return m, s.verify(m)
}

func (s Source) verify(m Manifest) error {
	sum, err := hex.DecodeString(m.SHA256)
	if err != nil || len(sum) != sha256.Size {
		return errors.New("manifest has no valid hash")
	}
	if m.Version == "" || m.URL == "" || m.Size <= 0 || m.Size > maxInstaller {
		return errors.New("manifest is incomplete")
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("manifest is not signed")
	}
	if !ed25519.Verify(s.PublicKey, SignedText(m.Version, m.SHA256), sig) {
		return errors.New("manifest signature does not match the publisher's key")
	}
	u, err := url.Parse(m.URL)
	if err != nil {
		return err
	}
	return s.allowed(u)
}

// Download fetches the installer of a verified manifest into dst. The file appears there
// only when its size and hash are the ones the publisher signed. progress may be nil.
func (s Source) Download(ctx context.Context, m Manifest, dst string, progress func(done, total int64)) error {
	if err := s.verify(m); err != nil {
		return err
	}
	resp, err := s.get(ctx, m.URL)
	if err != nil {
		return fmt.Errorf("fetch installer: %w", err)
	}
	defer resp.Body.Close()
	tmp := dst + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	hash := sha256.New()
	var done int64
	buf := make([]byte, 128<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			done += int64(n)
			if done > m.Size {
				rerr = errors.New("installer is larger than announced")
			} else if _, werr := f.Write(buf[:n]); werr != nil {
				rerr = werr
			} else {
				hash.Write(buf[:n])
				if progress != nil {
					progress(done, m.Size)
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("fetch installer: %w", rerr)
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if done != m.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), m.SHA256) {
		os.Remove(tmp)
		return errors.New("installer does not match the signed hash")
	}
	os.Remove(dst)
	return os.Rename(tmp, dst)
}

// Sign makes the signature of a manifest; the release tool uses it.
func Sign(key ed25519.PrivateKey, version, sha256hex string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, SignedText(version, sha256hex)))
}
