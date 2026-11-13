//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	uxtheme  = syscall.NewLazyDLL("uxtheme.dll")
	gdiplus  = syscall.NewLazyDLL("gdiplus.dll")

	pRegisterClassExW     = user32.NewProc("RegisterClassExW")
	pCreateWindowExW      = user32.NewProc("CreateWindowExW")
	pDefWindowProcW       = user32.NewProc("DefWindowProcW")
	pDestroyWindow        = user32.NewProc("DestroyWindow")
	pShowWindow           = user32.NewProc("ShowWindow")
	pUpdateWindow         = user32.NewProc("UpdateWindow")
	pGetMessageW          = user32.NewProc("GetMessageW")
	pTranslateMessage     = user32.NewProc("TranslateMessage")
	pDispatchMessageW     = user32.NewProc("DispatchMessageW")
	pIsDialogMessageW     = user32.NewProc("IsDialogMessageW")
	pPostQuitMessage      = user32.NewProc("PostQuitMessage")
	pPostMessageW         = user32.NewProc("PostMessageW")
	pSendMessageW         = user32.NewProc("SendMessageW")
	pMoveWindow           = user32.NewProc("MoveWindow")
	pGetClientRect        = user32.NewProc("GetClientRect")
	pLoadCursorW          = user32.NewProc("LoadCursorW")
	pLoadIconW            = user32.NewProc("LoadIconW")
	pSetFocus             = user32.NewProc("SetFocus")
	pGetFocus             = user32.NewProc("GetFocus")
	pGetKeyState          = user32.NewProc("GetKeyState")
	pSetWindowTextW       = user32.NewProc("SetWindowTextW")
	pGetWindowTextW       = user32.NewProc("GetWindowTextW")
	pGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	pGetForegroundWindow  = user32.NewProc("GetForegroundWindow")
	pSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	pIsWindowVisible      = user32.NewProc("IsWindowVisible")
	pIsIconic             = user32.NewProc("IsIconic")
	pCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	pAppendMenuW          = user32.NewProc("AppendMenuW")
	pTrackPopupMenu       = user32.NewProc("TrackPopupMenu")
	pDestroyMenu          = user32.NewProc("DestroyMenu")
	pGetCursorPos         = user32.NewProc("GetCursorPos")
	pGetDpiForWindow      = user32.NewProc("GetDpiForWindow")
	pSetProcessDpiCtx     = user32.NewProc("SetProcessDpiAwarenessContext")
	pFlashWindow          = user32.NewProc("FlashWindow")
	pSetTimer             = user32.NewProc("SetTimer")
	pKillTimer            = user32.NewProc("KillTimer")
	pInvalidateRect       = user32.NewProc("InvalidateRect")
	pMessageBoxW          = user32.NewProc("MessageBoxW")
	pBeginPaint           = user32.NewProc("BeginPaint")
	pEndPaint             = user32.NewProc("EndPaint")
	pFillRect             = user32.NewProc("FillRect")
	pDrawTextW            = user32.NewProc("DrawTextW")
	pDrawFocusRect        = user32.NewProc("DrawFocusRect")
	pSetScrollInfo        = user32.NewProc("SetScrollInfo")
	pGetScrollInfo        = user32.NewProc("GetScrollInfo")
	pGetDC                = user32.NewProc("GetDC")
	pReleaseDC            = user32.NewProc("ReleaseDC")
	pScreenToClient       = user32.NewProc("ScreenToClient")
	pEnableWindow         = user32.NewProc("EnableWindow")
	pSetWindowLongPtrW    = user32.NewProc("SetWindowLongPtrW")
	pCallWindowProcW      = user32.NewProc("CallWindowProcW")
	pTrackMouseEvent      = user32.NewProc("TrackMouseEvent")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pCreateActCtxW    = kernel32.NewProc("CreateActCtxW")
	pActivateActCtx   = kernel32.NewProc("ActivateActCtx")
	pGetSystemDirW    = kernel32.NewProc("GetSystemDirectoryW")
	pLoadLibraryW     = kernel32.NewProc("LoadLibraryW")
	pGetCurrentProc   = kernel32.NewProc("GetCurrentProcess")
	pGetProcMemInfo   = kernel32.NewProc("K32GetProcessMemoryInfo")

	pCreateFontW          = gdi32.NewProc("CreateFontW")
	pDeleteObject         = gdi32.NewProc("DeleteObject")
	pCreateCompatibleDC   = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBmp  = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject         = gdi32.NewProc("SelectObject")
	pDeleteDC             = gdi32.NewProc("DeleteDC")
	pBitBlt               = gdi32.NewProc("BitBlt")
	pCreateSolidBrush     = gdi32.NewProc("CreateSolidBrush")
	pSetBkMode            = gdi32.NewProc("SetBkMode")
	pSetBkColor           = gdi32.NewProc("SetBkColor")
	pSetTextColor         = gdi32.NewProc("SetTextColor")
	pGetTextExtentPoint32 = gdi32.NewProc("GetTextExtentPoint32W")
	pGetTextFaceW         = gdi32.NewProc("GetTextFaceW")

	pShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")

	pInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")

	pRegGetValueW       = advapi32.NewProc("RegGetValueW")
	pRegSetKeyValueW    = advapi32.NewProc("RegSetKeyValueW")
	pRegDeleteKeyValueW = advapi32.NewProc("RegDeleteKeyValueW")

	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	pSetWindowTheme        = uxtheme.NewProc("SetWindowTheme")

	pGdiplusStartup      = gdiplus.NewProc("GdiplusStartup")
	pGdipCreateFromHDC   = gdiplus.NewProc("GdipCreateFromHDC")
	pGdipDeleteGraphics  = gdiplus.NewProc("GdipDeleteGraphics")
	pGdipSetSmoothing    = gdiplus.NewProc("GdipSetSmoothingMode")
	pGdipSetPixelOffset  = gdiplus.NewProc("GdipSetPixelOffsetMode")
	pGdipCreateSolidFill = gdiplus.NewProc("GdipCreateSolidFill")
	pGdipDeleteBrush     = gdiplus.NewProc("GdipDeleteBrush")
	pGdipFillEllipseI    = gdiplus.NewProc("GdipFillEllipseI")
	pGdipCreatePath      = gdiplus.NewProc("GdipCreatePath")
	pGdipAddPathArcI     = gdiplus.NewProc("GdipAddPathArcI")
	pGdipClosePathFigure = gdiplus.NewProc("GdipClosePathFigure")
	pGdipFillPath        = gdiplus.NewProc("GdipFillPath")
	pGdipDeletePath      = gdiplus.NewProc("GdipDeletePath")
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	wsVScroll          = 0x00200000
	wsTabStop          = 0x00010000
	wsClipChildren     = 0x02000000

	esMultiline   = 0x0004
	esAutoVScroll = 0x0040
	esAutoHScroll = 0x0080
	esWantReturn  = 0x1000

	lbsNotify           = 0x0001
	lbsOwnerDrawFixed   = 0x0010
	lbsHasStrings       = 0x0040
	lbsNoIntegralHeight = 0x0100

	bsOwnerDraw = 0x000B

	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmActivate      = 0x0006
	wmSetFocus      = 0x0007
	wmKillFocus     = 0x0008
	wmSetRedraw     = 0x000B
	wmPaint         = 0x000F
	wmClose         = 0x0010
	wmEraseBkgnd    = 0x0014
	wmSettingChange = 0x001A
	wmGetMinMaxInfo = 0x0024
	wmDrawItem      = 0x002B
	wmMeasureItem   = 0x002C
	wmSetFont       = 0x0030
	wmGetDlgCode    = 0x0087
	wmKeyDown       = 0x0100
	wmCommand       = 0x0111
	wmTimer         = 0x0113
	wmVScroll       = 0x0115
	wmCtlColorEdit  = 0x0133
	wmCtlColorLB    = 0x0134
	wmMouseMove     = 0x0200
	wmLButtonDown   = 0x0201
	wmLButtonUp     = 0x0202
	wmRButtonUp     = 0x0205
	wmLButtonDblClk = 0x0203
	wmMouseWheel    = 0x020A
	wmMouseLeave    = 0x02A3
	wmDpiChanged    = 0x02E0
	wmApp           = 0x8000

	lbAddString     = 0x0180
	lbResetContent  = 0x0184
	lbSetCurSel     = 0x0186
	lbGetCurSel     = 0x0188
	lbGetTopIndex   = 0x018E
	lbSetTopIndex   = 0x0197
	lbGetItemRect   = 0x0198
	lbSetItemHeight = 0x01A0
	lbItemFromPoint = 0x01A9
	lbnSelChange    = 1

	emGetRect      = 0x00B2
	emLineScroll   = 0x00B6
	emScrollCaret  = 0x00B7
	emGetLineCount = 0x00BA
	emSetLimitText = 0x00C5
	emSetCueBanner = 0x1501
	enSetFocus     = 0x0100
	enKillFocus    = 0x0200
	enChange       = 0x0300

	odsSelected    = 0x0001
	odsDisabled    = 0x0004
	odsFocus       = 0x0010
	odsNoFocusRect = 0x0200

	dtCenter      = 0x0001
	dtRight       = 0x0002
	dtVCenter     = 0x0004
	dtWordBreak   = 0x0010
	dtSingleLine  = 0x0020
	dtCalcRect    = 0x0400
	dtNoPrefix    = 0x0800
	dtEditControl = 0x2000
	dtEndEllipsis = 0x8000

	sbVert       = 1
	sifRange     = 0x1
	sifPage      = 0x2
	sifPos       = 0x4
	sifTrackPos  = 0x10
	sbLineUp     = 0
	sbLineDown   = 1
	sbPageUp     = 2
	sbPageDown   = 3
	sbThumbTrack = 5
	sbTop        = 6
	sbBottom     = 7

	vkReturn  = 0x0D
	vkShift   = 0x10
	vkControl = 0x11
	vkEscape  = 0x1B
	vkSpace   = 0x20
	vkPrior   = 0x21
	vkNext    = 0x22
	vkEnd     = 0x23
	vkHome    = 0x24
	vkUp      = 0x26
	vkDown    = 0x28

	swHide    = 0
	swShow    = 5
	swRestore = 9

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2
	nifMsg    = 0x01
	nifIcon   = 0x02
	nifTip    = 0x04
	nifInfo   = 0x10
	niifInfo  = 0x01

	mfString     = 0x0000
	mfSeparator  = 0x0800
	tpmRightBtn  = 0x0002
	tpmReturnCmd = 0x0100
	tpmNoNotify  = 0x0080
	idiApp       = 32512
	idcArrow     = 32512
	iccStandard  = 0x4000

	actctxResName = 0x008
	actctxSetProc = 0x010
	actctxAsmDir  = 0x004

	hkcu       = 0x80000001
	rrfRtAny   = 0x0000FFFF
	regSz      = 1
	transparnt = 1
	srcCopy    = 0x00CC0020
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type point struct{ x, y int32 }

type msgT struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

type rect struct{ left, top, right, bottom int32 }

type paintStruct struct {
	hdc      uintptr
	erase    int32
	rcPaint  rect
	restore  int32
	incUpd   int32
	reserved [32]byte
}

type drawItem struct {
	ctlType    uint32
	ctlID      uint32
	itemID     uint32
	itemAction uint32
	itemState  uint32
	hwndItem   uintptr
	hDC        uintptr
	rcItem     rect
	itemData   uintptr
}

type measureItem struct {
	ctlType    uint32
	ctlID      uint32
	itemID     uint32
	itemWidth  uint32
	itemHeight uint32
	itemData   uintptr
}

type scrollInfo struct {
	cbSize uint32
	fMask  uint32
	nMin   int32
	nMax   int32
	nPage  uint32
	nPos   int32
	nTrack int32
}

type minMaxInfo struct {
	reserved, maxSize, maxPos, minTrack, maxTrack point
}

type sizeT struct{ cx, cy int32 }

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

type actCtx struct {
	cbSize                 uint32
	dwFlags                uint32
	lpSource               *uint16
	wProcessorArchitecture uint16
	wLangID                uint16
	lpAssemblyDirectory    *uint16
	lpResourceName         uintptr
	lpApplicationName      *uint16
	hModule                uintptr
}

type initCC struct {
	dwSize uint32
	dwICC  uint32
}

type procMemCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

func u16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("?")
	}
	return p
}

func u16s(s string) []uint16 {
	return syscall.StringToUTF16(sanitize(s))
}

func copyU16(dst []uint16, s string) {
	src := syscall.StringToUTF16(sanitize(s))
	if len(src) > len(dst) {
		src = src[:len(dst)]
		src[len(src)-1] = 0
	}
	copy(dst, src)
}

func sanitize(s string) string {
	b := []rune(s)
	for i, r := range b {
		if r == 0 {
			b[i] = ' '
		}
	}
	return string(b)
}

func sendMsg(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := pSendMessageW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func postMsg(hwnd uintptr, msg uint32, wp, lp uintptr) {
	pPostMessageW.Call(hwnd, uintptr(msg), wp, lp)
}

func setText(hwnd uintptr, s string) {
	pSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(u16(sanitize(s)))))
}

func getText(hwnd uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func createWindow(exStyle uint32, class, title string, style uint32, parent, id uintptr) uintptr {
	hinst, _, _ := pGetModuleHandleW.Call(0)
	h, _, _ := pCreateWindowExW.Call(uintptr(exStyle), uintptr(unsafe.Pointer(u16(class))),
		uintptr(unsafe.Pointer(u16(title))), uintptr(style), 0, 0, 0, 0, parent, id, hinst, 0)
	return h
}

func keyDown(vk int) bool {
	r, _, _ := pGetKeyState.Call(uintptr(vk))
	return int16(r) < 0
}

func invalidate(hwnd uintptr) {
	if hwnd != 0 {
		pInvalidateRect.Call(hwnd, 0, 0)
	}
}

func moveTo(h uintptr, r rect) {
	w, ht := r.right-r.left, r.bottom-r.top
	if w < 0 {
		w = 0
	}
	if ht < 0 {
		ht = 0
	}
	pMoveWindow.Call(h, uintptr(r.left), uintptr(r.top), uintptr(w), uintptr(ht), 1)
}

func show(h uintptr, on bool) {
	if on {
		pShowWindow.Call(h, swShow)
	} else {
		pShowWindow.Call(h, swHide)
	}
}

// regDword reads a DWORD from HKCU; def is returned when the value is missing.
func regDword(key, name string, def uint32) uint32 {
	var v, size uint32 = 0, 4
	r, _, _ := pRegGetValueW.Call(hkcu, uintptr(unsafe.Pointer(u16(key))), uintptr(unsafe.Pointer(u16(name))),
		rrfRtAny, 0, uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return def
	}
	return v
}

func regBytes(key, name string, max int) []byte {
	buf := make([]byte, max)
	size := uint32(max)
	r, _, _ := pRegGetValueW.Call(hkcu, uintptr(unsafe.Pointer(u16(key))), uintptr(unsafe.Pointer(u16(name))),
		rrfRtAny, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return nil
	}
	return buf[:size]
}

// procMemCountersEx2 is PROCESS_MEMORY_COUNTERS_EX2 (Windows 11 22H2 and later).
type procMemCountersEx2 struct {
	procMemCounters
	privateUsage          uintptr
	privateWorkingSetSize uintptr
	sharedCommitUsage     uint64
}

// workingSetMB returns the private working set, the figure Task Manager shows as "Memory".
// Older systems do not report it, so there the full working set is returned instead.
func workingSetMB() int {
	h, _, _ := pGetCurrentProc.Call()
	var ex procMemCountersEx2
	ex.cb = uint32(unsafe.Sizeof(ex))
	if r, _, _ := pGetProcMemInfo.Call(h, uintptr(unsafe.Pointer(&ex)), uintptr(ex.cb)); r != 0 && ex.privateWorkingSetSize != 0 {
		return int((ex.privateWorkingSetSize + 512*1024) / (1024 * 1024))
	}
	var pmc procMemCounters
	pmc.cb = uint32(unsafe.Sizeof(pmc))
	r, _, _ := pGetProcMemInfo.Call(h, uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.cb))
	if r == 0 {
		return 0
	}
	return int(pmc.workingSetSize / (1024 * 1024))
}

// enableVisualStyles activates common controls v6 without an embedded manifest.
func enableVisualStyles() {
	dir := make([]uint16, 260)
	n, _, _ := pGetSystemDirW.Call(uintptr(unsafe.Pointer(&dir[0])), 260)
	if n == 0 {
		return
	}
	ctx := actCtx{
		dwFlags:             actctxResName | actctxSetProc | actctxAsmDir,
		lpSource:            u16("shell32.dll"),
		lpAssemblyDirectory: &dir[0],
		lpResourceName:      124,
	}
	ctx.cbSize = uint32(unsafe.Sizeof(ctx))
	h, _, _ := pCreateActCtxW.Call(uintptr(unsafe.Pointer(&ctx)))
	if h == ^uintptr(0) || h == 0 {
		return
	}
	var cookie uintptr
	pActivateActCtx.Call(h, uintptr(unsafe.Pointer(&cookie)))
	icc := initCC{dwSize: 8, dwICC: iccStandard}
	pInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))
}
