package vector

import (
	"unsafe"
)

// unsafeBytesToInt8 casts a byte slice to an int8 slice without copying.
// The memory referenced by the return slice is the same as the input slice.
// If input is nil, nil is returned.
func unsafeBytesToInt8(b []byte) []int8 {
	if len(b) == 0 {
		return nil
	}
	return unsafe.Slice((*int8)(unsafe.Pointer(&b[0])), len(b))
}
