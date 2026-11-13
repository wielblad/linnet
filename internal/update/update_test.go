package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.2", "0.1", true}, {"0.1", "0.2", false}, {"0.1", "0.1", false},
		{"0.10", "0.9", true}, {"1.0", "0.99.9", true}, {"0.2.1", "0.2", true},
		{"v0.3", "0.2", true}, {"0.2-beta", "0.2", false}, {"", "0.1", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// server publishes one installer and its manifest; mutate changes the manifest first.
func server(t *testing.T, key ed25519.PrivateKey, installer, served []byte, mutate func(*Manifest)) (*httptest.Server, Manifest) {
	t.Helper()
	var m Manifest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest.json":
			json.NewEncoder(w).Encode(m)
		case "/setup.exe":
			w.Write(served)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(installer)
	m = Manifest{Version: "0.2", URL: srv.URL + "/setup.exe", Size: int64(len(installer)), SHA256: hex.EncodeToString(sum[:])}
	m.Signature = Sign(key, m.Version, m.SHA256)
	if mutate != nil {
		mutate(&m)
	}
	return srv, m
}

func TestCheckAndDownload(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(nil)
	installer := []byte(strings.Repeat("installer bytes ", 5000))
	srv, _ := server(t, key, installer, installer, nil)
	src := Source{ManifestURL: srv.URL + "/latest.json", PublicKey: pub, AllowLoopback: true}
	m, err := src.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "0.2" || !Newer(m.Version, "0.1") {
		t.Fatalf("manifest: %+v", m)
	}
	dst := filepath.Join(t.TempDir(), "setup.exe")
	var last int64
	if err := src.Download(context.Background(), m, dst, func(done, total int64) { last = done }); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(installer) || last != int64(len(installer)) {
		t.Fatalf("downloaded %d bytes, progress %d", len(got), last)
	}
}

func TestRejects(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(nil)
	_, otherKey, _ := ed25519.GenerateKey(nil)
	installer := []byte(strings.Repeat("x", 4096))
	ctx := context.Background()

	t.Run("signed with another key", func(t *testing.T) {
		srv, _ := server(t, otherKey, installer, installer, nil)
		if _, err := (Source{ManifestURL: srv.URL + "/latest.json", PublicKey: pub, AllowLoopback: true}).Check(ctx); err == nil {
			t.Fatal("accepted a manifest signed with a foreign key")
		}
	})
	t.Run("version changed after signing", func(t *testing.T) {
		srv, _ := server(t, key, installer, installer, func(m *Manifest) { m.Version = "9.9" })
		if _, err := (Source{ManifestURL: srv.URL + "/latest.json", PublicKey: pub, AllowLoopback: true}).Check(ctx); err == nil {
			t.Fatal("accepted a manifest whose version was changed")
		}
	})
	t.Run("installer replaced", func(t *testing.T) {
		tampered := append([]byte{}, installer...)
		tampered[100] = 'y'
		srv, m := server(t, key, installer, tampered, nil)
		dst := filepath.Join(t.TempDir(), "setup.exe")
		err := (Source{ManifestURL: srv.URL + "/latest.json", PublicKey: pub, AllowLoopback: true}).Download(ctx, m, dst, nil)
		if err == nil {
			t.Fatal("accepted a replaced installer")
		}
		if _, statErr := os.Stat(dst); statErr == nil {
			t.Fatal("a rejected installer was left on disk")
		}
	})
	t.Run("plain http refused without the test switch", func(t *testing.T) {
		srv, _ := server(t, key, installer, installer, nil)
		if _, err := (Source{ManifestURL: srv.URL + "/latest.json", PublicKey: pub}).Check(ctx); err == nil {
			t.Fatal("fetched a manifest over plain http")
		}
	})
}
