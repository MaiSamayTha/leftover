// Command sign makes and uses the key that signs Leftover releases, so the
// built-in updater can tell a real release from a tampered one. It is a
// release tool, not part of Leftover itself.
//
//	go run ./tools/sign keygen <private-key-file>   make a new key pair
//	go run ./tools/sign sign <file>                  write <file>.sig
//
// sign reads the private key, base64-encoded, from LEFTOVER_UPDATE_KEY. The
// public half goes in internal/update/keys.go.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sign:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: sign keygen <private-key-file> | sign sign <file>")
	}
	switch args[0] {
	case "keygen":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		seed := base64.StdEncoding.EncodeToString(priv.Seed())
		if err := os.WriteFile(args[1], []byte(seed+"\n"), 0o600); err != nil {
			return err
		}
		fmt.Println("private key written to", args[1])
		fmt.Println("public key:", base64.StdEncoding.EncodeToString(pub))
		return nil
	case "sign":
		raw := strings.TrimSpace(os.Getenv("LEFTOVER_UPDATE_KEY"))
		seed, err := base64.StdEncoding.DecodeString(raw)
		if err != nil || len(seed) != ed25519.SeedSize {
			return errors.New("LEFTOVER_UPDATE_KEY isn't a base64 Ed25519 private key")
		}
		msg, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		priv := ed25519.NewKeyFromSeed(seed)
		sig := ed25519.Sign(priv, msg)
		if !ed25519.Verify(priv.Public().(ed25519.PublicKey), msg, sig) {
			return errors.New("the signature didn't verify")
		}
		fmt.Println("public key:", base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)))
		return os.WriteFile(args[1]+".sig", []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644)
	}
	return fmt.Errorf("unknown command %q", args[0])
}
