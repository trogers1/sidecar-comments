package domain

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type NewIDRequest struct{}

var generator struct {
	sync.Mutex
	last [16]byte
}

func NewID(r NewIDRequest) (string, error) {
	generator.Lock()
	defer generator.Unlock()
	now := time.Now().UnixMilli()
	if now < 0 || now > 0xffffffffffff {
		return "", IO(IORequest{Operation: "clock", Path: "generator"})
	}
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", IO(IORequest{Operation: "csprng", Path: "generator"})
	}
	for i := 5; i >= 0; i-- {
		b[i] = byte(now)
		now >>= 8
	}
	b[6] = b[6]&15 | 0x70
	b[8] = b[8]&63 | 0x80
	if string(b[:]) <= string(generator.last[:]) {
		b = generator.last
		carry := true
		for i := 15; i >= 6 && carry; i-- {
			mask := byte(255)
			if i == 8 {
				mask = 63
			}
			if i == 6 {
				mask = 15
			}
			v := b[i] & mask
			if v < mask {
				b[i]++
				carry = false
			} else {
				b[i] &= ^mask
			}
		}
		if carry {
			return "", IO(IORequest{Operation: "clock", Path: "generator"})
		}
	}
	generator.last = b
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
