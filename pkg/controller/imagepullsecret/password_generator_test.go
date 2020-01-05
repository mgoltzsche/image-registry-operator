package imagepullsecret

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestGeneratePassword(t *testing.T) {
	pw := generatePassword()
	if len(pw) != 32 {
		t.Errorf("len != 32: %s", string(pw))
	}
	charCountMap := map[byte]int{}
	for _, c := range pw {
		charCountMap[c]++
	}
	if len(charCountMap) < 3 {
		t.Errorf("<3 various chars used: %s", string(pw))
	}
}

func TestBcryptPassword(t *testing.T) {
	pw := generatePassword()
	hashed, err := bcryptPassword(pw)
	if err != nil {
		t.Error(err)
	}
	if err = bcrypt.CompareHashAndPassword(hashed, pw); err != nil {
		t.Error(err)
	}
}
