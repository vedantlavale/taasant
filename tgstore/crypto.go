package tgstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

func newCipher(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func seal(aead cipher.AEAD, plain []byte) []byte {
	nonce := make([]byte, aead.NonceSize())
	rand.Read(nonce)
	return aead.Seal(nonce, nonce, plain, nil)
}

func open(aead cipher.AEAD, data []byte) ([]byte, error) {
	n := aead.NonceSize()
	if len(data) < n {
		return nil, errors.New("encrypted data is too short")
	}
	return aead.Open(nil, data[:n], data[n:], nil)
}
