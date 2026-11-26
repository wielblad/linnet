//go:build windows

package main

// Recording a voice message. The microphone is read through Windows' plain sound input,
// in pieces a tenth of a second long that the window collects as they fill. When the
// recording is finished, its samples are written as AAC sound in an MP4 file by Media
// Foundation: the kind of file a voice message is sent as.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

const (
	recRate    = 44100
	recPiece   = recRate / 10 * 2 // bytes of a tenth of a second: one channel, 16 bits
	recLongest = 10 * 60          // seconds; a longer one is cut there
	timerRec   = 32

	whdrDone = 0x1

	// IMFAttributes
	mfaSetUINT32 = 21
	// IMFSinkWriter
	mfwAddStream         = 3
	mfwSetInputMediaType = 4
	mfwBeginWriting      = 5
	mfwWriteSample       = 6
	mfwFinalize          = 11
	// IMFSample
	mfsSetSampleTime     = 36
	mfsSetSampleDuration = 38
	mfsAddBuffer         = 42
	// IMFMediaBuffer
	mfbSetCurrentLength = 6
)

var (
	pWaveInOpen            = winmm.NewProc("waveInOpen")
	pWaveInPrepareHeader   = winmm.NewProc("waveInPrepareHeader")
	pWaveInUnprepareHeader = winmm.NewProc("waveInUnprepareHeader")
	pWaveInAddBuffer       = winmm.NewProc("waveInAddBuffer")
	pWaveInStart           = winmm.NewProc("waveInStart")
	pWaveInReset           = winmm.NewProc("waveInReset")
	pWaveInClose           = winmm.NewProc("waveInClose")

	pMFCreateSinkWriterURL = mfreadwriteDLL.NewProc("MFCreateSinkWriterFromURL")
	pMFCreateSample        = mfplatDLL.NewProc("MFCreateSample")
	pMFCreateMemoryBuffer  = mfplatDLL.NewProc("MFCreateMemoryBuffer")

	mfAudioFormatAAC  = syscall.GUID{Data1: 0x00001610, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	mfAudioAvgBytes   = syscall.GUID{Data1: 0x1aab75c8, Data2: 0xcfef, Data3: 0x451c, Data4: [8]byte{0xab, 0x95, 0xac, 0x03, 0x4b, 0x8e, 0x17, 0x31}}
	mfAudioBlockAlign = syscall.GUID{Data1: 0x322de230, Data2: 0x9eeb, Data3: 0x43bd, Data4: [8]byte{0xab, 0x7a, 0xff, 0x41, 0x22, 0x51, 0x54, 0x1d}}

	rec struct {
		on      bool
		dev     uintptr
		hdrs    [4]waveHdr
		bufs    [4][]byte
		pcm     []byte
		started time.Time
		level   int32 // how loud the last piece was, 0 to 100
		host    uintptr
	}
)

type waveFormat struct {
	tag, channels   uint16
	rate, avgBytes  uint32
	align, bits, cb uint16
}

type waveHdr struct {
	data            uintptr
	length, written uint32
	user            uintptr
	flags, loops    uint32
	next, reserved  uintptr
}

// recStart opens the microphone and begins to record.
func recStart() error {
	if rec.on {
		return nil
	}
	wf := waveFormat{tag: 1, channels: 1, rate: recRate, avgBytes: recRate * 2, align: 2, bits: 16}
	var dev uintptr
	if r, _, _ := pWaveInOpen.Call(uintptr(unsafe.Pointer(&dev)), 0xFFFFFFFF, uintptr(unsafe.Pointer(&wf)), 0, 0, 0); r != 0 || dev == 0 {
		return fmt.Errorf("microphone: error %d", r)
	}
	rec.dev, rec.pcm, rec.level = dev, rec.pcm[:0], 0
	for i := range rec.hdrs {
		rec.bufs[i] = make([]byte, recPiece)
		rec.hdrs[i] = waveHdr{data: uintptr(unsafe.Pointer(&rec.bufs[i][0])), length: recPiece}
		pWaveInPrepareHeader.Call(dev, uintptr(unsafe.Pointer(&rec.hdrs[i])), unsafe.Sizeof(rec.hdrs[i]))
		pWaveInAddBuffer.Call(dev, uintptr(unsafe.Pointer(&rec.hdrs[i])), unsafe.Sizeof(rec.hdrs[i]))
	}
	if r, _, _ := pWaveInStart.Call(dev); r != 0 {
		recClose()
		return fmt.Errorf("microphone: cannot start, error %d", r)
	}
	rec.on, rec.started = true, time.Now()
	pSetTimer.Call(hMain, timerRec, 50, 0)
	return nil
}

// recCollect takes in the pieces that have filled and gives them back to be filled again.
func recCollect(again bool) {
	for i := range rec.hdrs {
		h := &rec.hdrs[i]
		if h.flags&whdrDone == 0 {
			continue
		}
		piece := rec.bufs[i][:min(h.written, recPiece)]
		rec.pcm = append(rec.pcm, piece...)
		peak := int32(0)
		for _, s := range unsafe.Slice((*int16)(unsafe.Pointer(&rec.bufs[i][0])), len(piece)/2) {
			v := int32(s)
			if v < 0 {
				v = -v
			}
			if v > peak {
				peak = v
			}
		}
		if len(piece) > 0 {
			rec.level = peak * 100 / 32768
		}
		h.flags &^= whdrDone
		h.written = 0
		if again {
			pWaveInAddBuffer.Call(rec.dev, uintptr(unsafe.Pointer(h)), unsafe.Sizeof(*h))
		}
	}
}

// recTick runs while a recording is made.
func recTick() {
	if !rec.on {
		pKillTimer.Call(hMain, timerRec)
		return
	}
	recCollect(true)
	if len(rec.pcm) >= recLongest*recRate*2 { // long enough: it waits to be sent or thrown away
		recPause()
	}
	if rec.host != 0 {
		invalidate(rec.host)
	}
}

// recPause stops the microphone and keeps what was recorded.
func recPause() {
	if rec.dev == 0 {
		return
	}
	pWaveInReset.Call(rec.dev) // every piece comes back, full or not
	recCollect(false)
	recClose()
}

func recClose() {
	if rec.dev != 0 {
		for i := range rec.hdrs {
			pWaveInUnprepareHeader.Call(rec.dev, uintptr(unsafe.Pointer(&rec.hdrs[i])), unsafe.Sizeof(rec.hdrs[i]))
		}
		pWaveInClose.Call(rec.dev)
		rec.dev = 0
	}
	pKillTimer.Call(hMain, timerRec)
}

// recSeconds is how long the recording is so far.
func recSeconds() int64 { return int64(len(rec.pcm)) / (recRate * 2) }

// recCancel throws the recording away.
func recCancel() {
	recPause()
	rec.on, rec.pcm = false, nil
}

// recFinish ends the recording and writes it to a file in the outbox. It gives the file,
// its length and its loudness from beginning to end, for the wave shown with it.
func recFinish() (path string, ms int64, wave []float64, err error) {
	recPause()
	pcm := rec.pcm
	rec.on, rec.pcm = false, nil
	if len(pcm) < recRate*2/2 { // less than half a second: a slip of the hand
		return "", 0, nil, errors.New("too short")
	}
	dir := filepath.Join(dataDir, "outbox")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", 0, nil, err
	}
	path = filepath.Join(dir, "glosowka-"+time.Now().Format("20060102-150405")+".m4a")
	if err := writeAAC(path, pcm, recRate); err != nil {
		os.Remove(path)
		return "", 0, nil, err
	}
	return path, int64(len(pcm)) * 1000 / (recRate * 2), pcmWave(pcm, 60), nil
}

// pcmWave gives the loudness of plain samples in n steps, each 0 to 1.
func pcmWave(pcm []byte, n int) []float64 {
	samples := unsafe.Slice((*int16)(unsafe.Pointer(&pcm[0])), len(pcm)/2)
	out := make([]float64, n)
	for i := range out {
		from, to := i*len(samples)/n, (i+1)*len(samples)/n
		peak := int32(0)
		for _, s := range samples[from:to] {
			v := int32(s)
			if v < 0 {
				v = -v
			}
			if v > peak {
				peak = v
			}
		}
		out[i] = float64(peak) / 32768
	}
	return out
}

// writeAAC writes plain samples (one channel, 16 bits) as AAC sound in an MP4 file.
func writeAAC(path string, pcm []byte, rate uint32) error {
	pCoInitializeEx.Call(0, 2)
	pMFStartup.Call(mfVersion, 0)
	var writer uintptr
	if hr, _, _ := pMFCreateSinkWriterURL.Call(uintptr(unsafe.Pointer(u16(path))), 0, 0, uintptr(unsafe.Pointer(&writer))); int32(hr) < 0 || writer == 0 {
		return fmt.Errorf("sound file: %08x", uint32(hr))
	}
	defer com(writer, 2)
	mediaType := func(subtype *syscall.GUID, extra map[*syscall.GUID]uint32) uintptr {
		var t uintptr
		pMFCreateMediaType.Call(uintptr(unsafe.Pointer(&t)))
		if t == 0 {
			return 0
		}
		com(t, mfaSetGUID, uintptr(unsafe.Pointer(&mfMajorType)), uintptr(unsafe.Pointer(&mfMediaTypeAudio)))
		com(t, mfaSetGUID, uintptr(unsafe.Pointer(&mfSubtype)), uintptr(unsafe.Pointer(subtype)))
		com(t, mfaSetUINT32, uintptr(unsafe.Pointer(&mfAudioBitsPerSmpl)), 16)
		com(t, mfaSetUINT32, uintptr(unsafe.Pointer(&mfAudioSamplesPerS)), uintptr(rate))
		com(t, mfaSetUINT32, uintptr(unsafe.Pointer(&mfAudioNumChannels)), 1)
		for k, v := range extra {
			com(t, mfaSetUINT32, uintptr(unsafe.Pointer(k)), uintptr(v))
		}
		return t
	}
	out := mediaType(&mfAudioFormatAAC, map[*syscall.GUID]uint32{&mfAudioAvgBytes: 12000}) // 96 kbit/s
	in := mediaType(&mfAudioFormatPCM, map[*syscall.GUID]uint32{&mfAudioAvgBytes: rate * 2, &mfAudioBlockAlign: 2})
	if out == 0 || in == 0 {
		return errors.New("sound file: no media type")
	}
	defer com(out, 2)
	defer com(in, 2)
	var stream uint32
	if hr := com(writer, mfwAddStream, out, uintptr(unsafe.Pointer(&stream))); int32(hr) < 0 {
		return fmt.Errorf("sound file: stream %08x", uint32(hr))
	}
	if hr := com(writer, mfwSetInputMediaType, uintptr(stream), in, 0); int32(hr) < 0 {
		return fmt.Errorf("sound file: input %08x", uint32(hr))
	}
	if hr := com(writer, mfwBeginWriting); int32(hr) < 0 {
		return fmt.Errorf("sound file: begin %08x", uint32(hr))
	}
	const chunk = recRate * 2 // a second at a time
	var at int64              // in units of 100 ns
	for off := 0; off < len(pcm); off += chunk {
		part := pcm[off:min(off+chunk, len(pcm))]
		var buf, sample uintptr
		pMFCreateMemoryBuffer.Call(uintptr(len(part)), uintptr(unsafe.Pointer(&buf)))
		pMFCreateSample.Call(uintptr(unsafe.Pointer(&sample)))
		if buf == 0 || sample == 0 {
			return errors.New("sound file: no memory")
		}
		var data uintptr
		com(buf, mfbLock, uintptr(unsafe.Pointer(&data)), 0, 0)
		copy(unsafe.Slice((*byte)(unsafe.Pointer(data)), len(part)), part)
		com(buf, mfbUnlock)
		com(buf, mfbSetCurrentLength, uintptr(len(part)))
		com(sample, mfsAddBuffer, buf)
		dur := int64(len(part)) * 10_000_000 / int64(rate*2)
		com(sample, mfsSetSampleTime, uintptr(at))
		com(sample, mfsSetSampleDuration, uintptr(dur))
		hr := com(writer, mfwWriteSample, uintptr(stream), sample)
		com(sample, 2)
		com(buf, 2)
		if int32(hr) < 0 {
			return fmt.Errorf("sound file: write %08x", uint32(hr))
		}
		at += dur
	}
	if hr := com(writer, mfwFinalize); int32(hr) < 0 {
		return fmt.Errorf("sound file: finish %08x", uint32(hr))
	}
	return nil
}
