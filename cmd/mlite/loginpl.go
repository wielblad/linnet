//go:build windows

package main

// Polish for what Meta's login steps say. The core passes on Meta's English; the known
// sentences are turned into Polish here, anything new is shown as it came.

import (
	"strings"

	"lazyfmess/internal/meta"
)

var plLoginText = map[string]string{
	"Choose how to finish signing in":                        "Wybierz, jak potwierdzić, że to Ty.",
	"Choose where to receive an MFA code":                    "Wybierz, gdzie wysłać kod weryfikacyjny.",
	"Choose the Facebook account to connect":                 "Wybierz konto Facebooka.",
	"Enter a six-digit code from your authenticator app":     "Wpisz sześciocyfrowy kod z aplikacji uwierzytelniającej.",
	"Enter one of your two-factor backup codes.":             "Wpisz jeden z kodów zapasowych.",
	"Enter the SMS code sent to your phone number":           "Wpisz kod z SMS-a wysłanego na Twój numer.",
	"Enter the code sent to you on WhatsApp":                 "Wpisz kod wysłany Ci na WhatsAppie.",
	"Enter characters":                                       "Przepisz znaki z obrazka.",
	"Check your notifications":                               "Sprawdź powiadomienia na innym urządzeniu.",
	"Open the notification":                                  "Otwórz powiadomienie na innym urządzeniu.",
	"Invalid username or password":                           "Nieprawidłowy login lub hasło.",
	"Invalid email address":                                  "Nieprawidłowy adres e-mail.",
	"Facebook rejected that login":                           "Facebook odrzucił to logowanie.",
	"Facebook rejected the login without providing a reason": "Facebook odrzucił logowanie bez podania powodu. Spróbuj ponownie albo zaloguj się najpierw w oficjalnej aplikacji lub na stronie.",
	"Meta can't send an SMS code right now":                  "Meta nie może teraz wysłać SMS-a. Spróbuj później albo wybierz inny sposób.",
	"Meta is requiring Google reCAPTCHA":                     "Meta wymaga testu reCAPTCHA, którego program nie obsługuje. Spróbuj później albo dokończ sprawdzenie w oficjalnej aplikacji lub na stronie.",
	"Meta is requiring Google sign-in":                       "Meta wymaga logowania przez Google, którego program nie obsługuje. Dodaj inny sposób weryfikacji w aplikacji lub na stronie Facebooka.",
	"Meta is requiring a passkey":                            "Meta wymaga klucza dostępu (passkey), którego program nie obsługuje. Dodaj inny sposób weryfikacji w aplikacji lub na stronie Facebooka.",
	"None of the available two-factor methods are supported": "Żaden z dostępnych sposobów weryfikacji nie jest obsługiwany. Dodaj inny w aplikacji lub na stronie Facebooka.",
}

var plOptions = map[string][2]string{
	"Notification on another device": {"Powiadomienie na innym urządzeniu", "Zatwierdź logowanie w aplikacji Facebook na telefonie"},
	"Authentication app":             {"Aplikacja uwierzytelniająca", "Kod z Google Authenticator, Microsoft Authenticator itp."},
	"Text message":                   {"SMS", "Kod wysłany SMS-em na Twój numer"},
	"Email":                          {"E-mail", "Kod wysłany na Twój adres e-mail"},
	"Backup code":                    {"Kod zapasowy", "Jeden z kodów zapisanych przy włączaniu weryfikacji"},
	"WhatsApp":                       {"WhatsApp", "Kod lub link wysłany na WhatsAppie"},
	"Re-enter password":              {"Wpisz hasło ponownie", ""},
}

// plLogin turns a sentence of a login step into Polish. Meta sometimes puts an error in front
// of the instruction ("Wrong code. Enter ..."), so each sentence is looked up on its own.
func plLogin(s string) string {
	if pl, ok := plLoginText[strings.TrimSpace(s)]; ok {
		return pl
	}
	parts := strings.Split(s, ". ")
	changed := false
	for i, p := range parts {
		key := strings.TrimSuffix(strings.TrimSpace(p), ".")
		for en, pl := range plLoginText {
			if strings.TrimSuffix(en, ".") == key {
				parts[i], changed = strings.TrimSuffix(pl, "."), true
			}
		}
	}
	if !changed {
		return s
	}
	out := strings.Join(parts, ". ")
	if !strings.HasSuffix(out, ".") {
		out += "."
	}
	return out
}

func optionLabel(v string) string {
	if pl, ok := plOptions[v]; ok {
		return pl[0]
	}
	return v
}

func optionAbout(v string) string {
	return plOptions[v][1]
}

// loginPreview shows a sign-in step without Meta, for looking at it: MLITE_LOGINPREVIEW
// with "choices", "code", "wait", "busy", "pin" or "pin-none". Nothing typed or chosen on
// such a screen goes anywhere.
func loginPreview(kind string) {
	switch kind {
	case "pin":
		historyAsk()
		return
	case "pin-none":
		historyNone("To konto nie ma kopii historii chronionej PIN-em, więc nie ma czego wczytać.")
		return
	}
	showLogin(false)
	lg.started = true
	switch kind {
	case "choices":
		lg.step = meta.LoginStep{Kind: meta.LoginInput, Instructions: "Choose how to finish signing in",
			Fields: []meta.LoginField{{ID: "mfatype", Name: "Login method", Options: []string{
				"Notification on another device", "Authentication app", "Text message", "Email", "Backup code"}}}}
	case "code":
		lg.step = meta.LoginStep{Kind: meta.LoginInput, Instructions: "Enter a six-digit code from your authenticator app",
			Fields: []meta.LoginField{{ID: "totp_code", Name: "Six-digit code"}}}
	case "wait":
		lg.step = meta.LoginStep{Kind: meta.LoginWait}
	default:
		lg.busy = true
		lg.step = meta.LoginStep{Kind: meta.LoginInput, Fields: []meta.LoginField{
			{ID: "username", Name: "Username"}, {ID: "password", Name: "Password", Secret: true}}}
	}
	loginFields()
	layout()
}

