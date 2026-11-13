// Command release prepares a release of Linnet for the updater.
//
//	release keygen                      makes the publisher's signing key (once)
//	release sign -version 0.2 -url URL  signs build/Linnet-Setup.exe and writes dist/
//
// The private key stays in .local/keys (never committed); its public half is written into
// cmd/mlite/updatekey.go and travels inside the program. -url is the address the
// installer will be downloaded from.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"lazyfmess/internal/update"
)

const (
	keyFile = ".local/keys/update-ed25519.key"
	pubFile = "cmd/mlite/updatekey.go"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: release keygen | release sign -version V -url URL [-exe FILE] [-notes TEXT] [-out DIR]")
	}
	switch os.Args[1] {
	case "keygen":
		keygen()
	case "sign":
		sign(os.Args[2:])
	default:
		fail("unknown command " + os.Args[1])
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func keygen() {
	if _, err := os.Stat(keyFile); err == nil {
		fail("a signing key already exists at " + keyFile + "; installed programs trust it, so it is not replaced")
	}
	pub, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		fail(err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o700); err != nil {
		fail(err.Error())
	}
	if err := os.WriteFile(keyFile, []byte(hex.EncodeToString(key.Seed())+"\n"), 0o600); err != nil {
		fail(err.Error())
	}
	src := "//go:build windows\n\npackage main\n\n// updatePublicKey is the publisher's key: updates are accepted only when signed with its\n// private half (written by `release keygen`, kept outside the repository).\nconst updatePublicKey = \"" + hex.EncodeToString(pub) + "\"\n"
	if err := os.WriteFile(pubFile, []byte(src), 0o644); err != nil {
		fail(err.Error())
	}
	fmt.Println("signing key written to", keyFile, "- keep a copy somewhere safe")
	fmt.Println("public key written to", pubFile)
}

func sign(args []string) {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	version := fs.String("version", "", "version being released")
	base := fs.String("url", "", "address the installer will be downloaded from")
	exe := fs.String("exe", "build/Linnet-Setup.exe", "installer to sign")
	notes := fs.String("notes", "", "what is new")
	out := fs.String("out", "dist", "folder for the files to publish")
	fs.Parse(args)
	if *version == "" || *base == "" {
		fail("-version and -url are required")
	}
	seedHex, err := os.ReadFile(keyFile)
	if err != nil {
		fail("no signing key: run `release keygen` first (" + err.Error() + ")")
	}
	seed, err := hex.DecodeString(string(trim(seedHex)))
	if err != nil || len(seed) != ed25519.SeedSize {
		fail("signing key file is damaged")
	}
	key := ed25519.NewKeyFromSeed(seed)
	data, err := os.ReadFile(*exe)
	if err != nil {
		fail(err.Error())
	}
	sum := sha256.Sum256(data)
	m := update.Manifest{Version: *version, URL: *base, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Notes: *notes}
	m.Signature = update.Sign(key, m.Version, m.SHA256)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err.Error())
	}
	if err := os.WriteFile(filepath.Join(*out, "Linnet-Setup.exe"), data, 0o755); err != nil {
		fail(err.Error())
	}
	raw, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(*out, "latest.json"), append(raw, '\n'), 0o644); err != nil {
		fail(err.Error())
	}
	fmt.Printf("release %s: %d bytes, sha256 %s\nfiles to publish in %s: Linnet-Setup.exe, latest.json\n", m.Version, m.Size, m.SHA256, *out)
}

func trim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}
