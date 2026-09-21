package face

import (
	"encoding/binary"
	"fmt"
	"math"
)

func Encode(feature []float32) []byte {
	raw := make([]byte, len(feature)*4)
	for i, value := range feature {
		binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(value))
	}
	return raw
}

func Decode(raw []byte) ([]float32, error) {
	if len(raw) == 0 || len(raw)%4 != 0 {
		return nil, fmt.Errorf("invalid embedding length %d", len(raw))
	}
	feature := make([]float32, len(raw)/4)
	for i := range feature {
		feature[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return feature, nil
}
