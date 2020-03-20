package imagepullsecret

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand"

	"golang.org/x/crypto/bcrypt"
)

const (
	digitChars   = "0123456789"
	specialChars = "=.+-_/[]{}@?"
	allChars     = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		digitChars + specialChars
)

func bcryptPassword(password []byte) (p []byte, err error) {
	return bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
}

func generatePassword() []byte {
	var src cryptoSource
	rnd := rand.New(src)
	length := 16
	buf := make([]byte, length)
	buf[0] = digitChars[rnd.Intn(len(digitChars))]
	buf[1] = specialChars[rnd.Intn(len(specialChars))]
	for i := 2; i < length; i++ {
		buf[i] = allChars[rand.Intn(len(allChars))]
	}
	rnd.Shuffle(len(buf), func(i, j int) {
		buf[i], buf[j] = buf[j], buf[i]
	})
	return buf
}

type cryptoSource struct{}

func (s cryptoSource) Seed(seed int64) {}

func (s cryptoSource) Int63() int64 {
	return int64(s.Uint64() & ^uint64(1<<63))
}

func (s cryptoSource) Uint64() (v uint64) {
	err := binary.Read(crand.Reader, binary.BigEndian, &v)
	if err != nil {
		panic(err)
	}
	return v
}
