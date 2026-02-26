package main

/*
#cgo CFLAGS: -I${SRCDIR}/ten-vad/include

// macOS (Darwin)
#cgo darwin CFLAGS: -I${SRCDIR}/ten-vad/lib/macOS/ten_vad.framework/Versions/A/Headers
#cgo darwin LDFLAGS: -F${SRCDIR}/ten-vad/lib/macOS -framework ten_vad -Wl,-rpath,${SRCDIR}/ten-vad/lib/macOS

// Linux AMD64
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/ten-vad/lib/Linux/x64 -lten_vad -Wl,-rpath,'$ORIGIN'/ten-vad/lib/Linux/x64

// Windows AMD64
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/ten-vad/lib/Windows/x64 -lten_vad

#include "ten_vad.h"
#include <stdlib.h>
#include <stddef.h>
#include <stdint.h>
*/
import "C"
import (
	"fmt"
	"runtime"
	"unsafe"
)

type Vad struct {
	instance C.ten_vad_handle_t
	hopSize  int
}

func NewVad(hopSize int, threshold float32) (*Vad, error) {
	var inst C.ten_vad_handle_t

	ret := C.ten_vad_create(&inst, C.size_t(hopSize), C.float(threshold))
	if ret != 0 || inst == nil {
		return nil, fmt.Errorf("ten_vad_create failed (code %d)", ret)
	}

	v := &Vad{instance: inst, hopSize: hopSize}
	runtime.SetFinalizer(v, func(vad *Vad) {
		if vad.instance != nil {
			C.ten_vad_destroy(&vad.instance)
			vad.instance = nil
		}
	})
	return v, nil
}

func (v *Vad) Close() {
	if v.instance != nil {
		C.ten_vad_destroy(&v.instance)
		v.instance = nil
		runtime.SetFinalizer(v, nil)
	}
}

// Process runs VAD on a single frame of int16 PCM samples.
// Returns speech probability, whether speech was detected, and any error.
func (v *Vad) Process(frame []int16) (float32, bool, error) {
	if v.instance == nil {
		return 0, false, fmt.Errorf("vad: instance closed")
	}
	if len(frame) != v.hopSize {
		return 0, false, fmt.Errorf("vad: frame length %d != hop_size %d", len(frame), v.hopSize)
	}

	var prob C.float
	var flag C.int

	ret := C.ten_vad_process(v.instance, (*C.short)(unsafe.Pointer(&frame[0])), C.size_t(v.hopSize), &prob, &flag)
	if ret != 0 {
		return 0, false, fmt.Errorf("ten_vad_process failed (code %d)", ret)
	}
	return float32(prob), flag == 1, nil
}
