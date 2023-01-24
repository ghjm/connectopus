// Package cmrand provides the friendly API of math.rand, using crypto.rand as its source.
package cmrand

import (
	crand "crypto/rand"
	"encoding/binary"
	log "github.com/sirupsen/logrus"
	"math/rand"
)

var r *rand.Rand

func Rand() *rand.Rand {
	if r == nil {
		var src cryptoSource
		r = rand.New(src)
	}
	return r
}

type cryptoSource struct{}

func (s cryptoSource) Seed(seed int64) {}

func (s cryptoSource) Int63() int64 {
	return int64(s.Uint64() & ^uint64(1<<63))
}

func (s cryptoSource) Uint64() (v uint64) {
	err := binary.Read(crand.Reader, binary.BigEndian, &v)
	if err != nil {
		log.Fatal(err)
	}
	return v
}
