package meb

import (
	"encoding/binary"
	"math"
)

// Metadata encoding version
const MetadataVersion1 = 1

// EncodeFactMetadata encodes the metadata (Weight, Source) into a byte slice.
// Format: [Version:1][Weight:8][Source...]
func EncodeFactMetadata(f Fact) []byte {
	// Source bytes
	sourceBytes := []byte(f.Source)

	// Total size: 1 (version) + 8 (weight) + len(source)
	buf := make([]byte, 1+8+len(sourceBytes))

	buf[0] = MetadataVersion1

	// Encode Weight (float64)
	binary.BigEndian.PutUint64(buf[1:9], math.Float64bits(f.Weight))

	// Copy Source
	copy(buf[9:], sourceBytes)

	return buf
}

// DecodeFactMetadata extracts Weight and Source from a byte slice.
// Returns defaults (1.0, "") if data is empty or invalid.
func DecodeFactMetadata(data []byte) (float64, string) {
	if len(data) == 0 {
		return 1.0, "" // Default values
	}

	// Check version
	if data[0] != MetadataVersion1 {
		// Unknown version, return defaults (or best effort?)
		return 1.0, ""
	}

	if len(data) < 9 {
		// Not enough data for weight
		return 1.0, ""
	}

	// Decode Weight
	bits := binary.BigEndian.Uint64(data[1:9])
	weight := math.Float64frombits(bits)

	// Decode Source
	source := string(data[9:])

	return weight, source
}
