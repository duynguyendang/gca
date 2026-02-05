package vector

import (
	"fmt"
	"os"
	"syscall"
)

// loadMmap maps a file into memory and returns the byte slice.
func loadMmap(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	size := info.Size()
	if size == 0 {
		return nil, nil
	}

	data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmap failed: %w", err)
	}

	return data, nil
}

// unloadMmap unmaps the memory.
func unloadMmap(data []byte) error {
	if data == nil {
		return nil
	}
	return syscall.Munmap(data)
}
