//go:build windows

package main

import "testing"

func TestPlLogin(t *testing.T) {
	for in, want := range map[string]string{
		"Choose how to finish signing in":                                                  "Wybierz, jak potwierdzić, że to Ty.",
		"Invalid username or password. Enter a six-digit code from your authenticator app": "Nieprawidłowy login lub hasło. Wpisz sześciocyfrowy kod z aplikacji uwierzytelniającej.",
		"Something new from Meta":                                                          "Something new from Meta",
	} {
		if got := plLogin(in); got != want {
			t.Errorf("plLogin(%q) = %q, want %q", in, got, want)
		}
	}
	if optionLabel("Text message") != "SMS" || optionLabel("Carrier pigeon") != "Carrier pigeon" {
		t.Error("option labels")
	}
}
