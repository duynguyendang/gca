package common

const (
	FNV1aOffsetBasis uint32 = 2166136261
	FNV1aPrime       uint32 = 16777619
)

func FNV1aHash(data string) uint32 {
	var h uint32 = FNV1aOffsetBasis
	for i := 0; i < len(data); i++ {
		h ^= uint32(data[i])
		h *= FNV1aPrime
	}
	return h
}
