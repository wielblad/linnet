//go:build windows

package main

// The wave of a voice recording: how loud it is from its beginning to its end, as a row of
// heights. It is measured from the recording itself: Windows' Media Foundation turns the
// file into plain samples, and the loudest sample of each short stretch is kept. Nothing
// is invented: a file that cannot be read has no wave and keeps its line of words.

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	waveSlots = 96 // heights kept for one recording; drawn with as many bars as fit

	mfVersion          = 0x00020070
	mfFirstAudioStream = 0xFFFFFFFD
	mfEndOfStream      = 0x2

	// IMFAttributes (IMFMediaType is one): after the three of IUnknown
	mfaGetUINT32 = 7
	mfaSetGUID   = 24
	// IMFSourceReader
	mfrGetCurrentMediaType = 6
	mfrSetCurrentMediaType = 7
	mfrReadSample          = 9
	// IMFSample, after the thirty of IMFAttributes
	mfsConvertToContiguous = 41
	// IMFMediaBuffer
	mfbLock   = 3
	mfbUnlock = 4
)

var (
	mfplatDLL          = syscall.NewLazyDLL("mfplat.dll")
	mfreadwriteDLL     = syscall.NewLazyDLL("mfreadwrite.dll")
	pMFStartup         = mfplatDLL.NewProc("MFStartup")
	pMFCreateMediaType = mfplatDLL.NewProc("MFCreateMediaType")
	pMFCreateReaderURL = mfreadwriteDLL.NewProc("MFCreateSourceReaderFromURL")
	mfMajorType        = syscall.GUID{Data1: 0x48eba18e, Data2: 0xf8c9, Data3: 0x4687, Data4: [8]byte{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}}
	mfSubtype          = syscall.GUID{Data1: 0xf7e34c9a, Data2: 0x42e8, Data3: 0x4714, Data4: [8]byte{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}}
	mfMediaTypeAudio   = syscall.GUID{Data1: 0x73647561, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	mfAudioFormatPCM   = syscall.GUID{Data1: 0x00000001, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	mfAudioBitsPerSmpl = syscall.GUID{Data1: 0xf2deb57f, Data2: 0x40fa, Data3: 0x4764, Data4: [8]byte{0xaa, 0x33, 0xed, 0x4f, 0x2d, 0x1f, 0xf6, 0x69}}
	mfAudioSamplesPerS = syscall.GUID{Data1: 0x5faeeae7, Data2: 0x0290, Data3: 0x4c31, Data4: [8]byte{0x9e, 0x8a, 0xc5, 0x34, 0xf6, 0x8d, 0x9d, 0xba}}
	mfAudioNumChannels = syscall.GUID{Data1: 0x37e48bf5, Data2: 0x645e, Data3: 0x4c5b, Data4: [8]byte{0x89, 0xde, 0xad, 0xa9, 0xe2, 0x9b, 0x69, 0x6a}}
	waves              sync.Map // message -> []uint8 of waveSlots heights (0..255), or nil when it has none
	waveBusy           sync.Map
	waveOnce           sync.Once
	waveQueue          = make(chan [2]string, 64)
)

// waveOf gives the wave of a recording when it has been measured.
func waveOf(id string) []uint8 {
	if w, ok := waves.Load(id); ok {
		h, _ := w.([]uint8)
		return h
	}
	return nil
}

// waveKick asks for the wave of a recording whose file is on the disk. It is measured once,
// away from the window, one recording after another.
func waveKick(id, path string) {
	if _, done := waves.Load(id); done {
		return
	}
	if _, busy := waveBusy.LoadOrStore(id, true); busy {
		return
	}
	waveOnce.Do(func() { go waveWorker() })
	select {
	case waveQueue <- [2]string{id, path}:
	default:
		waveBusy.Delete(id) // too many at once: asked again when it is next drawn
	}
}

func waveWorker() {
	runtime.LockOSThread()
	pCoInitializeEx.Call(0, 0)
	pMFStartup.Call(mfVersion, 0)
	for job := range waveQueue {
		waves.Store(job[0], measureWave(playableSound(job[1])))
		waveBusy.Delete(job[0])
		be.requestRefresh() // drawn where the recording is on the screen
	}
}

// measureWave reads a sound file to its end and gives waveSlots heights, the loudest one
// stretched to 255. It gives nil for a file that cannot be read or is silent.
func measureWave(path string) []uint8 {
	var reader uintptr
	if hr, _, _ := pMFCreateReaderURL.Call(uintptr(unsafe.Pointer(u16(path))), 0, uintptr(unsafe.Pointer(&reader))); int32(hr) < 0 || reader == 0 {
		return nil
	}
	defer com(reader, 2)
	var want uintptr
	if hr, _, _ := pMFCreateMediaType.Call(uintptr(unsafe.Pointer(&want))); int32(hr) < 0 || want == 0 {
		return nil
	}
	com(want, mfaSetGUID, uintptr(unsafe.Pointer(&mfMajorType)), uintptr(unsafe.Pointer(&mfMediaTypeAudio)))
	com(want, mfaSetGUID, uintptr(unsafe.Pointer(&mfSubtype)), uintptr(unsafe.Pointer(&mfAudioFormatPCM)))
	hr := com(reader, mfrSetCurrentMediaType, mfFirstAudioStream, 0, want)
	com(want, 2)
	if int32(hr) < 0 {
		return nil
	}
	var got uintptr
	if int32(com(reader, mfrGetCurrentMediaType, mfFirstAudioStream, uintptr(unsafe.Pointer(&got)))) < 0 || got == 0 {
		return nil
	}
	var bits, rate, chans uint32
	com(got, mfaGetUINT32, uintptr(unsafe.Pointer(&mfAudioBitsPerSmpl)), uintptr(unsafe.Pointer(&bits)))
	com(got, mfaGetUINT32, uintptr(unsafe.Pointer(&mfAudioSamplesPerS)), uintptr(unsafe.Pointer(&rate)))
	com(got, mfaGetUINT32, uintptr(unsafe.Pointer(&mfAudioNumChannels)), uintptr(unsafe.Pointer(&chans)))
	com(got, 2)
	if bits != 16 || rate == 0 || chans == 0 {
		return nil
	}
	// the loudest sample of every 20 ms; an hour of sound is the most that is looked at
	per := int(rate) * int(chans) / 50
	var peaks []uint16
	peak, n := uint16(0), 0
	for len(peaks) < 50*3600 {
		var flags uint32
		var sample uintptr
		if int32(com(reader, mfrReadSample, mfFirstAudioStream, 0, 0, uintptr(unsafe.Pointer(&flags)), 0, uintptr(unsafe.Pointer(&sample)))) < 0 {
			break
		}
		if sample != 0 {
			var buf uintptr
			if int32(com(sample, mfsConvertToContiguous, uintptr(unsafe.Pointer(&buf)))) >= 0 && buf != 0 {
				var data uintptr
				var size uint32
				if int32(com(buf, mfbLock, uintptr(unsafe.Pointer(&data)), 0, uintptr(unsafe.Pointer(&size)))) >= 0 && data != 0 {
					for _, s := range unsafe.Slice((*int16)(unsafe.Pointer(data)), int(size)/2) {
						v := uint16(s)
						if s < 0 {
							v = uint16(-int32(s))
						}
						if v > peak {
							peak = v
						}
						if n++; n >= per {
							peaks = append(peaks, peak)
							peak, n = 0, 0
						}
					}
					com(buf, mfbUnlock)
				}
				com(buf, 2)
			}
			com(sample, 2)
		}
		if flags&mfEndOfStream != 0 {
			break
		}
	}
	return waveHeights(peaks)
}

// waveHeights brings the loudness of a recording, stretch by stretch, to waveSlots heights.
func waveHeights(peaks []uint16) []uint8 {
	if len(peaks) == 0 {
		return nil
	}
	out := make([]uint8, waveSlots)
	top := uint16(0)
	slot := make([]uint16, waveSlots)
	for i := range slot {
		from, to := i*len(peaks)/waveSlots, (i+1)*len(peaks)/waveSlots
		if to <= from {
			to = from + 1
		}
		for _, p := range peaks[from:min(to, len(peaks))] {
			if p > slot[i] {
				slot[i] = p
			}
		}
		if slot[i] > top {
			top = slot[i]
		}
	}
	if top < 64 { // silence
		return nil
	}
	for i, p := range slot {
		out[i] = uint8(uint32(p) * 255 / uint32(top))
	}
	return out
}
