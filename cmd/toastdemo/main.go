//go:build windows

// toastdemo: shows one toast notification, to try internal/toast.
//
//	toastdemo [-title T] [-body B] [-image PNG] [-reply] [-silent] [-seconds N] [-echo]
//
// It waits for a click or a sent reply, prints it as "activated args=<...> textlen=<n>"
// and exits; without one it exits after -seconds. The toast is taken back on exit.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"lazyfmess/internal/toast"
)

const (
	appID = "Linnet" // the AppUserModelID of the installed program's Start Menu shortcut
	tag   = "toastdemo"
	group = "demo"
)

func main() {
	title := flag.String("title", "Test powiadomienia", "first line of the toast")
	body := flag.String("body", "To jest test.", "second line of the toast")
	image := flag.String("image", "", "path of a PNG shown round in place of the app logo")
	reply := flag.Bool("reply", false, "add a text box with a send button")
	silent := flag.Bool("silent", false, "no sound")
	seconds := flag.Int("seconds", 25, "how long to wait for an activation before exiting")
	echo := flag.Bool("echo", false, "also print the reply text, not only its length")
	flag.Parse()

	// The handler runs on a system thread: it only passes the activation on to main.
	activations := make(chan toast.Activation, 8)
	err := toast.Start(appID, func(a toast.Activation) {
		select {
		case activations <- a:
		default:
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := 0
	err = toast.Show(toast.Notification{
		Title: *title, Body: *body, Image: *image,
		Tag: tag, Group: group,
		Args:  "body-click",
		Reply: *reply, ReplyHint: "Odpowiedz…", ReplyLabel: "Wyślij", ReplyArgs: "reply-sent",
		Silent: *silent,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	} else {
		fmt.Println("shown")
		select {
		case a := <-activations:
			fmt.Printf("activated args=%s textlen=%d\n", a.Args, len([]rune(a.Text)))
			if *echo {
				fmt.Printf("text=%s\n", a.Text)
			}
		case <-time.After(time.Duration(*seconds) * time.Second):
			fmt.Println("no activation")
		}
	}
	toast.Remove(tag, group)
	toast.Stop()
	os.Exit(code)
}
