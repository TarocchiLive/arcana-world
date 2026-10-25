// SPDX-License-Identifier: GPL-3.0-only

package edgetts

import (
	"encoding/binary"
	"math/bits"
)

// GEC v2 算法移植自 rany2/edge-tts 的 drm.py（ba61d94442c18893f0c4d3c5ad253718b21ab93e）。
// 首块从预计算的第 14 轮状态恢复，不能用标准 SHA-256 替代。
var gecInitial = [8]uint32{0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A, 0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19}
var gecState = [8]uint32{0xFD9FBEA2, 0x91243CE7, 0xBE499094, 0xCD2E1E0A, 0x6C006F49, 0x04BD35AB, 0xFEB0A7E4, 0x03EFFBD2}
var gecSchedule = [14]uint32{0xEB24BF9E, 0x1558691F, 0xA0C09EE2, 0x0CFA32BA, 0xA622EE8C, 0xD1DD9491, 0xC0DD25A0, 0xF9E70AB6, 0xF0C31573, 0x7058CE4E, 0x7188E606, 0x00E5AB33, 0xD258B060, 0x72766564}
var gecRounds = [64]uint32{
	0x428A2F98, 0x71374491, 0xB5C0FBCF, 0xE9B5DBA5, 0x3956C25B, 0x59F111F1, 0x923F82A4, 0xAB1C5ED5,
	0xD807AA98, 0x12835B01, 0x243185BE, 0x550C7DC3, 0x72BE5D74, 0x80DEB1FE, 0x9BDC06A7, 0xC19BF174,
	0xE49B69C1, 0xEFBE4786, 0x0FC19DC6, 0x240CA1CC, 0x2DE92C6F, 0x4A7484AA, 0x5CB0A9DC, 0x76F988DA,
	0x983E5152, 0xA831C66D, 0xB00327C8, 0xBF597FC7, 0xC6E00BF3, 0xD5A79147, 0x06CA6351, 0x14292967,
	0x27B70A85, 0x2E1B2138, 0x4D2C6DFC, 0x53380D13, 0x650A7354, 0x766A0ABB, 0x81C2C92E, 0x92722C85,
	0xA2BFE8A1, 0xA81A664B, 0xC24B8B70, 0xC76C51A3, 0xD192E819, 0xD6990624, 0xF40E3585, 0x106AA070,
	0x19A4C116, 0x1E376C08, 0x2748774C, 0x34B0BCB5, 0x391C0CB3, 0x4ED8AA4A, 0x5B9CCA4F, 0x682E6FF3,
	0x748F82EE, 0x78A5636F, 0x84C87814, 0x8CC70208, 0x90BEFFFA, 0xA4506CEB, 0xBEF9A3F7, 0xC67178F2,
}

// 以下修正常量与上游同名常量对应：首块从第 14 轮恢复时替换第 14 轮常量，
// 并修正部分扩展字；CS64 的系数与偏移同样取自上游。
const (
	gecAdjustment       = 0xC9DC06A7 // _SHA256_ADJUSTMENT
	gecSigma0Adjustment = 0x059C0B80 // _SHA256_SIGMA0_ADJUSTMENT
	gecSigma1Adjustment = 0x000B92C0 // _SHA256_SIGMA1_ADJUSTMENT
	gecCS64Factor       = 0x82893933 // _CS64_FACTOR
	gecCS64Adjustment0  = 0xF2F90B1A // _CS64_ADJUSTMENT0
	gecCS64Adjustment1  = 0x0B1A0000 // _CS64_ADJUSTMENT1
	// 首块前 57 字节是上游折算进 gecState 与 gecSchedule 的固定前缀，随后 7 字节取自 data。
	gecPrefixBytes = 57
)

func gecSigma0(v uint32) uint32 {
	return bits.RotateLeft32(v, -7) ^ bits.RotateLeft32(v, -18) ^ (v >> 3)
}

func gecSigma1(v uint32) uint32 {
	return bits.RotateLeft32(v, -17) ^ bits.RotateLeft32(v, -19) ^ (v >> 10)
}

func gecExpand(w *[64]uint32, start int) {
	for i := start; i < len(w); i++ {
		w[i] = gecSigma1(w[i-2]) + w[i-7] + gecSigma0(w[i-15]) + w[i-16]
	}
}

func gecCompress(state [8]uint32, w *[64]uint32, start int) [8]uint32 {
	a, b, c, d, e, f, g, h := state[0], state[1], state[2], state[3], state[4], state[5], state[6], state[7]
	for i := start; i < 64; i++ {
		k := gecRounds[i]
		if start == 14 && i == 14 {
			k = gecAdjustment
		}
		t1 := h + (bits.RotateLeft32(e, -6) ^ bits.RotateLeft32(e, -11) ^ bits.RotateLeft32(e, -25)) + (g ^ (e & (f ^ g))) + k + w[i]
		t2 := (bits.RotateLeft32(a, -2) ^ bits.RotateLeft32(a, -13) ^ bits.RotateLeft32(a, -22)) + ((a & b) | (c & (a | b)))
		a, b, c, d, e, f, g, h = t1+t2, a, b, c, d+t1, e, f, g
	}
	return [8]uint32{a, b, c, d, e, f, g, h}
}

// gecSHA256 的 data 是文件时间十进制串与客户端令牌（固定 50 字节）；
// 长度须为 7..71 字节，上限由 gecCS64 的填充缓冲决定。
func gecSHA256(data []byte) [32]byte {
	var w [64]uint32
	w[14] = uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
	w[15] = binary.BigEndian.Uint32(data[3:7])
	for i := 16; i < 30; i++ {
		v := gecSchedule[i-16]
		s := gecSigma1(w[i-2])
		if i == 16 {
			s ^= gecSigma1Adjustment
		}
		v += s
		if i >= 21 {
			v += w[i-7]
		}
		if i >= 29 {
			v += gecSigma0(w[i-15]) ^ gecSigma0Adjustment
		}
		w[i] = v
	}
	w[30] = gecSigma1(w[28]) + w[23] + gecSigma0(w[15]) + w[14] + gecAdjustment - gecRounds[14]
	gecExpand(&w, 31)
	state := gecCompress(gecState, &w, 14)
	for i := range state {
		state[i] += gecInitial[i]
	}
	// 首块已消费前缀与七字节 data；剩余数据及 SHA 填充最多占两个块。
	var padded [128]byte
	n := copy(padded[:], data[7:])
	padded[n] = 0x80
	size := (n + 9 + 63) / 64 * 64
	binary.BigEndian.PutUint64(padded[size-8:size], uint64(gecPrefixBytes+len(data))*8)
	for offset := 0; offset < size; offset += 64 {
		for i := range 16 {
			w[i] = binary.BigEndian.Uint32(padded[offset+i*4:])
		}
		gecExpand(&w, 16)
		working := gecCompress(state, &w, 0)
		for i := range state {
			state[i] += working[i]
		}
	}
	var digest [32]byte
	for i, v := range state {
		binary.BigEndian.PutUint32(digest[i*4:], v)
	}
	return digest
}

var gecCoefficients = [14][2]uint32{
	{0xE82E91B1, 0xB32700D0}, {0x44DE24C6, 0x88DFF51D}, {0x3BEB65BC, 0x29514038}, {0xC89C5933, 0xB328CBDF},
	{0x8D1EC211, 0xD405B4D5}, {0x314EF5D5, 0x26E55F3C}, {0xE92491B1, 0x607801C6}, {0xF0A5B9D5, 0x82B2C1FB},
	{0x06064CF3, 0xFA51A3DA}, {0xDA79FA60, 0x207CC464}, {0xBED20560, 0x3C3C74F3}, {0x89F9C9F8, 0xD2D14B61},
	{0x35FA38C6, 0xBB04A2C1}, {0xCB49E3C6, 0xC5626167},
}

func gecCS64(data []byte, digest [32]byte) [8]byte {
	keys := [2]uint32{binary.LittleEndian.Uint32(digest[:4]) | 1, binary.LittleEndian.Uint32(digest[4:8]) | 1}
	factors := [2]uint32{keys[0] * gecCS64Factor, keys[1] * gecCS64Factor}
	var terms, offsets [32]uint32
	for i, pair := range gecCoefficients {
		terms[i], offsets[i] = pair[0]*factors[i%2], pair[1]
	}
	var padded [72]byte
	copy(padded[1:], data)
	words := (len(data) + 1 + 7) / 8 * 2
	for i := range words {
		word := binary.LittleEndian.Uint32(padded[i*4:])
		factor := uint32(0xEF0569FB)
		if i%2 != 0 {
			factor = 0xC31713DB
		}
		terms[14+i] = word * keys[i%2]
		offsets[14+i] = bits.RotateLeft32(word, 16) * factor
		if i == 0 {
			terms[14+i] += gecCS64Adjustment0 * factors[0]
			offsets[14+i] += gecCS64Adjustment1
		}
	}
	var swapAcc, swapLast, state, acc uint32
	for i := 0; i < 14+words; i += 2 {
		x, y := terms[i], terms[i+1]
		q := bits.RotateLeft32(offsets[i]+x, 16) * 0x76F50F34
		swapAcc += bits.RotateLeft32(q, 16)*0xC3EFEA97 + q
		q = bits.RotateLeft32(offsets[i+1]+y, 16) * 0x1881CE3C
		swapLast = bits.RotateLeft32(q, 16)*0x35BD1EC9 + q
		swapAcc += swapLast
		state = state*keys[0] + x
		state = bits.RotateLeft32(state, 16) * 0xCF98B111
		state = bits.RotateLeft32(state, 16) * 0x87085B9F
		state = bits.RotateLeft32(state, 16) * 0x12CEB96D
		state = bits.RotateLeft32(state, 16) * 0x257E1D83
		acc += state
		t := state*keys[1] + y
		t = bits.RotateLeft32(t, 16) * 0xA27416F5
		t = bits.RotateLeft32(t, 16) * 0xD38396FF
		state = bits.RotateLeft32(bits.RotateLeft32(t, 16)*0x7C932B89, 16) * 0xBFA49F69
		acc += state
	}
	var result [8]byte
	binary.LittleEndian.PutUint32(result[:4], swapLast^state)
	binary.LittleEndian.PutUint32(result[4:], swapAcc^acc)
	return result
}
