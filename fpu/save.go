package fpu

import (
	"encoding/binary"
	"errors"
)

// ErrFrameFormat reports an FRESTORE frame the FPU does not accept. The CPU
// takes it as a format error.
var ErrFrameFormat = errors.New("fpu: invalid FRESTORE frame")

// FSAVE frames start with a long: version (bits 31-24), size of the rest of
// the frame in bytes (bits 23-16) and a reserved word. A null frame (version
// 0) means the FPU is in its reset state; an idle frame holds the internal
// state between instructions.
const idleVersion = 0x1f

// idleSize returns the size of the idle frame after its header.
func (f *FPU) idleSize() int {
	if f.model == MC68882 {
		return 0x38
	}
	return 0x18
}

// FrameHeader returns the first long of the frame Save would produce.
func (f *FPU) FrameHeader() uint32 {
	if f.null {
		return 0
	}
	return idleVersion<<24 | uint32(f.idleSize())<<16
}

// Save returns the FSAVE frame: a null frame in the reset state, an idle
// frame otherwise. The idle frame records a pending exception in its first
// byte; the rest is zero.
func (f *FPU) Save() []byte {
	frame := binary.BigEndian.AppendUint32(nil, f.FrameHeader())
	if f.null {
		return frame
	}
	state := make([]byte, f.idleSize())
	state[0] = f.pending
	return append(frame, state...)
}

// Restore loads an FSAVE frame. A null frame resets the FPU; an idle frame
// of the model's size restores its pending exception.
func (f *FPU) Restore(frame []byte) error {
	if len(frame) < 4 {
		return ErrFrameFormat
	}
	header := binary.BigEndian.Uint32(frame)
	switch {
	case header>>24 == 0:
		f.Reset()
		return nil
	case header>>24 == idleVersion && int(header>>16&0xff) == f.idleSize() && len(frame) == 4+f.idleSize():
		f.null = false
		f.pending = frame[4]
		return nil
	}
	return ErrFrameFormat
}
