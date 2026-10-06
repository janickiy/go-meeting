package captions

import (
	"encoding/binary"
	"math"
)

// AudioActive оценивает энергию PCM, не выдавая открытый микрофон за произнесённую речь.
// @args pcm — mono signed PCM16LE.
// @return true при RMS выше -42 dBFS; шум и музыка могут давать ложное срабатывание.
func AudioActive(pcm []byte) bool {
	var sum float64
	n := len(pcm) / 2
	if n == 0 {
		return false
	}
	for i := 0; i+1 < len(pcm); i += 2 {
		v := float64(int16(binary.LittleEndian.Uint16(pcm[i:i+2]))) / 32768
		sum += v * v
	}
	return math.Sqrt(sum/float64(n)) >= 0.007943282
}
