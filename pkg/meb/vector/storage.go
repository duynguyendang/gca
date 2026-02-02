package vector

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"

	"github.com/dgraph-io/badger/v4"
)

// persistFullVector writes the full 1536-d vector to BadgerDB.
// Key format: "vec:full:<BigEndianID>"
func (r *VectorRegistry) persistFullVector(id uint64, fullVec []float32) error {
	key := make([]byte, 1+8)
	key[0] = 0x10 // Prefix for full vectors
	binary.BigEndian.PutUint64(key[1:9], id)

	// Serialize vector to bytes (little-endian for performance)
	value := make([]byte, FullDim*4)
	for i, v := range fullVec {
		binary.LittleEndian.PutUint32(value[i*4:(i+1)*4], math.Float32bits(v))
	}

	return r.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

// GetFullVector retrieves the full 1536-d vector from disk.
func (r *VectorRegistry) GetFullVector(id uint64) ([]float32, error) {
	key := make([]byte, 1+8)
	key[0] = 0x10 // Prefix for full vectors
	binary.BigEndian.PutUint64(key[1:9], id)

	var fullVec []float32
	err := r.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			fullVec = make([]float32, FullDim)
			for i := 0; i < FullDim; i++ {
				bits := binary.LittleEndian.Uint32(val[i*4 : (i+1)*4])
				fullVec[i] = math.Float32frombits(bits)
			}
			return nil
		})
	})

	return fullVec, err
}

// SaveSnapshot persists the current RAM state to BadgerDB.
func (r *VectorRegistry) SaveSnapshot() error {
	// Wait for all async writes to complete
	r.wg.Wait()

	r.mu.Lock()
	defer r.mu.Unlock()

	numVectors := len(r.revMap)
	slog.Info("saving vector snapshot",
		"vectorCount", numVectors,
		"dataSizeBytes", len(r.data),
	)

	// Serialize int8 vectors (direct byte copy - very fast)
	vectorsBytes := make([]byte, len(r.data))
	for i, v := range r.data {
		vectorsBytes[i] = byte(v)
	}

	// Serialize revMap
	idsBytes := make([]byte, len(r.revMap)*8)
	for i, id := range r.revMap {
		binary.BigEndian.PutUint64(idsBytes[i*8:(i+1)*8], id)
	}

	// Serialize stringIDs (for semantic search symbol lookup)
	var stringIDsBytes []byte
	for _, sid := range r.stringIDs {
		idBytes := []byte(sid)
		// Write length as 4-byte big-endian
		lenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBytes, uint32(len(idBytes)))
		stringIDsBytes = append(stringIDsBytes, lenBytes...)
		stringIDsBytes = append(stringIDsBytes, idBytes...)
	}

	batch := r.db.NewWriteBatch()
	defer batch.Cancel()

	// Save vectors snapshot
	if err := batch.Set([]byte("sys:mrl:vectors"), vectorsBytes); err != nil {
		slog.Error("failed to save vectors snapshot", "error", err)
		return fmt.Errorf("failed to save vectors snapshot: %w", err)
	}

	// Save IDs snapshot
	if err := batch.Set([]byte("sys:mrl:ids"), idsBytes); err != nil {
		slog.Error("failed to save IDs snapshot", "error", err)
		return fmt.Errorf("failed to save IDs snapshot: %w", err)
	}

	// Save stringIDs snapshot
	if len(stringIDsBytes) > 0 {
		if err := batch.Set([]byte("sys:mrl:string_ids"), stringIDsBytes); err != nil {
			slog.Error("failed to save stringIDs snapshot", "error", err)
			return fmt.Errorf("failed to save stringIDs snapshot: %w", err)
		}
	}

	if err := batch.Flush(); err != nil {
		slog.Error("failed to flush snapshot batch", "error", err)
		return err
	}

	slog.Info("vector snapshot saved successfully",
		"vectorCount", numVectors,
		"vectorsSizeBytes", len(vectorsBytes),
		"idsSizeBytes", len(idsBytes),
	)

	return nil
}

// LoadSnapshot restores the RAM state from BadgerDB.
func (r *VectorRegistry) LoadSnapshot() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	slog.Info("loading vector snapshot")

	var vectorsBytes, idsBytes, stringIDsBytes []byte

	err := r.db.View(func(txn *badger.Txn) error {
		// Load vectors
		item, err := txn.Get([]byte("sys:mrl:vectors"))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				// No snapshot exists
				slog.Info("no existing vector snapshot found")
				return nil
			}
			slog.Error("failed to load vectors snapshot", "error", err)
			return fmt.Errorf("failed to load vectors snapshot: %w", err)
		}

		if err := item.Value(func(val []byte) error {
			vectorsBytes = make([]byte, len(val))
			copy(vectorsBytes, val)
			return nil
		}); err != nil {
			return err
		}

		// Load IDs
		item, err = txn.Get([]byte("sys:mrl:ids"))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			slog.Error("failed to load IDs snapshot", "error", err)
			return fmt.Errorf("failed to load IDs snapshot: %w", err)
		}

		if err := item.Value(func(val []byte) error {
			idsBytes = make([]byte, len(val))
			copy(idsBytes, val)
			return nil
		}); err != nil {
			return err
		}

		// Load stringIDs (for semantic search)
		item, err = txn.Get([]byte("sys:mrl:string_ids"))
		if err != nil {
			if err != badger.ErrKeyNotFound {
				slog.Warn("failed to load stringIDs snapshot", "error", err)
			}
			return nil // Not fatal, just no string IDs
		}

		return item.Value(func(val []byte) error {
			stringIDsBytes = make([]byte, len(val))
			copy(stringIDsBytes, val)
			return nil
		})
	})

	if err != nil {
		return err
	}

	// No snapshot found
	if vectorsBytes == nil || idsBytes == nil {
		return nil
	}

	// Deserialize int8 vectors (direct byte copy - very fast)
	numVectors := len(vectorsBytes) / MRLDim
	r.data = make([]int8, numVectors*MRLDim)
	for i, v := range vectorsBytes {
		r.data[i] = int8(v)
	}

	// Deserialize revMap
	r.revMap = make([]uint64, numVectors)
	for i := 0; i < numVectors; i++ {
		r.revMap[i] = binary.BigEndian.Uint64(idsBytes[i*8 : (i+1)*8])
	}

	// Rebuild idMap
	r.idMap = make(map[uint64]uint32, numVectors)
	for idx, id := range r.revMap {
		r.idMap[id] = uint32(idx)
	}

	// Deserialize stringIDs (if present)
	r.stringIDs = make([]string, 0, numVectors)
	if stringIDsBytes != nil {
		offset := 0
		for offset < len(stringIDsBytes) {
			if offset+4 > len(stringIDsBytes) {
				break
			}
			strLen := binary.BigEndian.Uint32(stringIDsBytes[offset : offset+4])
			offset += 4
			if offset+int(strLen) > len(stringIDsBytes) {
				break
			}
			r.stringIDs = append(r.stringIDs, string(stringIDsBytes[offset:offset+int(strLen)]))
			offset += int(strLen)
		}
	}

	slog.Info("vector snapshot loaded successfully",
		"vectorCount", numVectors,
		"vectorsSizeBytes", len(vectorsBytes),
		"idsSizeBytes", len(idsBytes),
	)

	return nil
}
