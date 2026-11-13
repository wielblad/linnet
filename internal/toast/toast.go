//go:build windows

// Package toast shows Windows toast notifications, optionally with a quick-reply text
// box, from an unpackaged Win32 program using nothing but the standard library: no cgo,
// raw WinRT calls through COM vtables.
//
// The program needs an AppUserModelID that Windows knows: a Start Menu shortcut carrying
// it. Toasts appear under that shortcut's name and icon. A click or a sent reply comes
// back in-process, through the Activated event of the toast object, and therefore only
// while the process that showed the toast is alive and has not called Remove or Stop for
// it; no COM activator is registered.
//
// All WinRT work is done on one thread owned by the package, which sits in the
// multithreaded apartment, so the functions here may be called from any goroutine.
package toast

import (
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"
)

// maxKept is how many shown toasts stay activatable at once. Each one is a live WinRT
// object; the oldest is let go when a new one would exceed the number.
const maxKept = 20

// Notification is one toast. Reply adds a text box with a send button.
type Notification struct {
	Title, Body string
	Image       string // optional path of a PNG shown round, as the app logo override
	Tag, Group  string // identify the toast so that a newer one replaces it and it can be removed
	Args        string // passed back when the body of the toast is clicked
	Reply       bool
	ReplyHint   string // placeholder of the text box
	ReplyLabel  string // label of the send button
	ReplyArgs   string // passed back, with the text, when the reply is sent
	Silent      bool   // no sound
}

// Activation is what the user did with a toast.
type Activation struct {
	Args string // Args of the notification, or ReplyArgs when a reply was sent
	Text string // the reply typed; empty for a click on the body
}

var (
	errNotStarted = errors.New("toast: not started")

	// lock is held shared by every call sent to the worker and exclusively by Start and
	// Stop, so the queue never goes away under a call.
	lock    sync.RWMutex
	queue   chan func(*session)
	stopped chan struct{} // closed when the worker has released everything
)

// session is the state of the worker thread between Start and Stop. Only that thread
// touches it.
type session struct {
	appID    uintptr // HSTRING
	notifier uintptr // IToastNotifier
	factory  uintptr // IToastNotificationFactory
	history  uintptr // IToastNotificationHistory; 0 when Windows gave none
	handler  uintptr // the Activated delegate
	kept     []kept  // oldest first
}

// kept is a toast that may still be activated: the event only fires while its object and
// the registration live.
type kept struct {
	notification uintptr // IToastNotification
	token        int64   // registration of the Activated handler
	tag, group   string
}

// Start prepares toasts for the given AppUserModelID. Activations are delivered by calling
// notify from an arbitrary thread; the caller forwards them to its own thread itself.
//
// notify runs on a system thread that Windows is waiting on: it must return quickly and
// must not call Stop.
func Start(appID string, notify func(Activation)) error {
	if appID == "" {
		return errors.New("toast: no AppUserModelID")
	}
	lock.Lock()
	defer lock.Unlock()
	if queue != nil {
		return errors.New("toast: already started")
	}
	q := make(chan func(*session))
	done := make(chan struct{})
	ready := make(chan error, 1)
	go work(appID, q, ready, done)
	if err := <-ready; err != nil {
		<-done
		return err
	}
	if notify != nil {
		receiver.Store(&notify)
	}
	queue, stopped = q, done
	return nil
}

// Show puts a toast on the screen and into the notification centre. A toast with the Tag
// and Group of an earlier one takes its place.
func Show(n Notification) error {
	return call(func(s *session) error { return s.show(n) })
}

// Remove takes a toast back from the screen and the notification centre. With an empty
// tag it takes back every toast of the group; with both empty it does nothing.
//
// Give toasts a Group as well as a Tag: Windows' history removes nothing without a group
// (E_INVALIDARG), so a toast with a Tag alone can only be taken back while it is among
// the last maxKept shown.
func Remove(tag, group string) {
	call(func(s *session) error { s.remove(tag, group); return nil })
}

// Stop lets go of everything Start and Show made. Toasts still on the screen or in the
// notification centre stay there, but can no longer report a click; Remove them first to
// take them away. It waits for calls in progress.
func Stop() {
	lock.Lock()
	defer lock.Unlock()
	if queue == nil {
		return
	}
	receiver.Store(nil)
	close(queue)
	<-stopped
	queue, stopped = nil, nil
}

// call runs f on the worker thread and waits for it.
func call(f func(*session) error) error {
	lock.RLock()
	defer lock.RUnlock()
	if queue == nil {
		return errNotStarted
	}
	result := make(chan error, 1)
	queue <- func(s *session) { result <- f(s) }
	return <-result
}

// work is the worker: one OS thread that initialises WinRT, makes every call and cleans
// up. The thread ends with the goroutine, because it stays locked.
func work(appID string, q <-chan func(*session), ready chan<- error, done chan<- struct{}) {
	defer close(done)
	runtime.LockOSThread()
	owed, err := roInit()
	if err != nil {
		ready <- err
		return
	}
	if owed {
		defer pRoUninitialize.Call()
	}
	s := &session{}
	defer s.close()
	if err := s.open(appID); err != nil {
		ready <- err
		return
	}
	ready <- nil
	for f := range q {
		f(s)
	}
}

func (s *session) open(appID string) error {
	var err error
	if s.appID, err = newHString(appID); err != nil {
		return err
	}
	manager, err := activationFactory(classToastNotificationManager, &iidManagerStatics)
	if err != nil {
		return err
	}
	defer comRelease(&manager)
	if hr := comCall(manager, managerCreateNotifierWithID, s.appID, uintptr(unsafe.Pointer(&s.notifier))); hr < 0 || s.notifier == 0 {
		return hresultError("CreateToastNotifierWithId", hr)
	}
	if s.factory, err = activationFactory(classToastNotification, &iidFactory); err != nil {
		return err
	}
	// Without the history toasts can still be shown and hidden, so its absence is no error.
	if manager2 := queryInterface(manager, &iidManagerStatics2); manager2 != 0 {
		comCall(manager2, manager2GetHistory, uintptr(unsafe.Pointer(&s.history)))
		comRelease(&manager2)
	}
	s.handler = activatedHandlerAddr()
	return nil
}

func (s *session) close() {
	for i := range s.kept {
		s.release(&s.kept[i])
	}
	s.kept = nil
	comRelease(&s.history)
	comRelease(&s.factory)
	comRelease(&s.notifier)
	deleteHString(s.appID)
	s.appID = 0
}

// release unregisters the handler from a kept toast and lets its object go.
func (s *session) release(k *kept) {
	if k.notification != 0 {
		removeToken(k.notification, notificationRemoveActivated, k.token)
		comRelease(&k.notification)
	}
}

func (s *session) show(n Notification) error {
	xml, err := newHString(toastXML(n))
	if err != nil {
		return err
	}
	defer deleteHString(xml)

	object, err := activateInstance(classXMLDocument)
	if err != nil {
		return err
	}
	defer comRelease(&object)
	io := queryInterface(object, &iidXMLDocumentIO)
	if io == 0 {
		return hresultError("QueryInterface(IXmlDocumentIO)", hresult(eNoInterface))
	}
	hr := comCall(io, xmlDocumentIOLoadXML, xml)
	comRelease(&io)
	if hr < 0 {
		return hresultError("LoadXml", hr)
	}
	document := queryInterface(object, &iidXMLDocument)
	if document == 0 {
		return hresultError("QueryInterface(IXmlDocument)", hresult(eNoInterface))
	}
	defer comRelease(&document)

	var notification uintptr
	if hr := comCall(s.factory, factoryCreate, document, uintptr(unsafe.Pointer(&notification))); hr < 0 || notification == 0 {
		return hresultError("CreateToastNotification", hr)
	}
	k := kept{notification: notification, tag: n.Tag, group: n.Group}
	if n.Tag != "" || n.Group != "" {
		if err := setTagAndGroup(notification, n.Tag, n.Group); err != nil {
			comRelease(&notification)
			return err
		}
	}
	if hr := comCall(notification, notificationAddActivated, s.handler, uintptr(unsafe.Pointer(&k.token))); hr < 0 {
		comRelease(&notification)
		return hresultError("add_Activated", hr)
	}
	if hr := comCall(s.notifier, notifierShow, notification); hr < 0 {
		s.release(&k)
		return hresultError("Show", hr)
	}

	// The toast this one replaced on the screen can no longer be activated.
	if n.Tag != "" {
		s.drop(func(old *kept) bool { return old.tag == n.Tag && old.group == n.Group })
	}
	for len(s.kept) >= maxKept {
		s.release(&s.kept[0])
		s.kept = append(s.kept[:0], s.kept[1:]...)
	}
	s.kept = append(s.kept, k)
	return nil
}

func setTagAndGroup(notification uintptr, tag, group string) error {
	n2 := queryInterface(notification, &iidNotification2)
	if n2 == 0 {
		return hresultError("QueryInterface(IToastNotification2)", hresult(eNoInterface))
	}
	defer comRelease(&n2)
	for _, p := range []struct {
		step  string
		slot  uintptr
		value string
	}{{"put_Tag", notification2PutTag, tag}, {"put_Group", notification2PutGroup, group}} {
		if p.value == "" {
			continue
		}
		h, err := newHString(p.value)
		if err != nil {
			return err
		}
		hr := comCall(n2, p.slot, h)
		deleteHString(h)
		if hr < 0 {
			return hresultError(p.step, hr)
		}
	}
	return nil
}

// drop releases the kept toasts that match and takes them off the list.
func (s *session) drop(match func(*kept) bool) {
	rest := s.kept[:0]
	for i := range s.kept {
		if match(&s.kept[i]) {
			s.release(&s.kept[i])
			continue
		}
		rest = append(rest, s.kept[i])
	}
	for i := len(rest); i < len(s.kept); i++ {
		s.kept[i] = kept{}
	}
	s.kept = rest
}

func (s *session) remove(tag, group string) {
	if tag == "" && group == "" {
		return
	}
	match := func(k *kept) bool { return k.group == group && (tag == "" || k.tag == tag) }
	// Hide works on the objects still held, whatever their tag and group; the history
	// then clears the notification centre, also of toasts no longer held.
	for i := range s.kept {
		if match(&s.kept[i]) {
			comCall(s.notifier, notifierHide, s.kept[i].notification)
		}
	}
	s.drop(match)
	if s.history == 0 || group == "" {
		return
	}
	g, err := newHString(group)
	if err != nil {
		return
	}
	defer deleteHString(g)
	if tag == "" {
		comCall(s.history, historyRemoveGroupWithID, g, s.appID)
		return
	}
	t, err := newHString(tag)
	if err != nil {
		return
	}
	defer deleteHString(t)
	comCall(s.history, historyRemoveGroupedTagWithID, t, g, s.appID)
}

// toastXML builds the toast's content.
func toastXML(n Notification) string {
	var b strings.Builder
	b.WriteString(`<toast launch="` + escape(n.Args) + `"><visual><binding template="ToastGeneric">`)
	b.WriteString(`<text>` + escape(n.Title) + `</text><text>` + escape(n.Body) + `</text>`)
	if n.Image != "" {
		b.WriteString(`<image placement="appLogoOverride" hint-crop="circle" src="` + escape(fileURI(n.Image)) + `"/>`)
	}
	b.WriteString(`</binding></visual>`)
	if n.Reply {
		b.WriteString(`<actions><input id="` + replyInputID + `" type="text" placeHolderContent="` + escape(n.ReplyHint) + `"/>`)
		b.WriteString(`<action content="` + escape(n.ReplyLabel) + `" arguments="` + escape(n.ReplyArgs) +
			`" hint-inputId="` + replyInputID + `" activationType="foreground"/></actions>`)
	}
	if n.Silent {
		b.WriteString(`<audio silent="true"/>`)
	}
	b.WriteString(`</toast>`)
	return b.String()
}

// escape makes s safe as XML text and as an attribute value. Characters XML 1.0 cannot
// carry at all are left out.
func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '"':
			b.WriteString("&quot;")
		case r == '\'':
			b.WriteString("&apos;")
		case r == '\t':
			b.WriteString("&#9;")
		case r == '\n':
			b.WriteString("&#10;")
		case r == '\r':
			b.WriteString("&#13;")
		case r < 0x20, r == 0xFFFE, r == 0xFFFF:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fileURI turns a file path into a file: address.
func fileURI(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	path = filepath.ToSlash(path)
	u := url.URL{Scheme: "file", Path: "/" + path}
	if share, ok := strings.CutPrefix(path, "//"); ok { // \\server\share\file
		host, rest, _ := strings.Cut(share, "/")
		u.Host, u.Path = host, "/"+rest
	}
	return u.String()
}
