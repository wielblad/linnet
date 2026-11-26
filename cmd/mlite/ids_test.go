//go:build windows

package main

import "testing"

// The buttons of the window are told apart by their numbers: two with one number answer to
// each other's clicks.
func TestControlNumbersDiffer(t *testing.T) {
	ids := map[string]int{
		"idSearch": idSearch, "idList": idList, "idInput": idInput, "idSend": idSend, "idAttach": idAttach,
		"idFind": idFind, "idInfo": idInfo, "idClose": idClose, "idApp": idApp, "idMic": idMic, "idGif": idGif,
		"idNavChats": idNavChats, "idNavPeople": idNavPeople, "idNavSettings": idNavSettings,
		"idLoginNext": idLoginNext, "idLoginCancel": idLoginCancel, "idLoginEye": idLoginEye, "idPanelEdit": idPanelEdit,
	}
	seen := map[int]string{}
	for name, id := range ids {
		if other, ok := seen[id]; ok {
			t.Errorf("%s and %s share the number %d", name, other, id)
		}
		seen[id] = name
	}
}
