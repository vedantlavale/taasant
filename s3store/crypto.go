package s3store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
)

func newCipher(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// An object is a row of chunks, each sealed on its own. What a chunk is
// sealed with names the object, its place in the row and whether it is the
// end, so whoever can write to the bucket cannot swap, drop, repeat or cut
// short the chunks without being found out.
func sealChunk(aead cipher.AEAD, plain []byte, object string, index int, last bool) []byte {
	nonce := make([]byte, aead.NonceSize())
	rand.Read(nonce)
	return aead.Seal(nonce, nonce, plain, binding(object, index, last))
}

func openChunk(aead cipher.AEAD, data []byte, object string, index int, last bool) ([]byte, error) {
	n := aead.NonceSize()
	if len(data) < n {
		return nil, errors.New("encrypted data is too short")
	}
	plain, err := aead.Open(nil, data[:n], data[n:], binding(object, index, last))
	if err != nil {
		return nil, errors.New("a chunk could not be decrypted: the key is wrong, or the file was changed or cut short")
	}
	return plain, nil
}

func binding(object string, index int, last bool) []byte {
	data := binary.BigEndian.AppendUint64([]byte(object), uint64(index))
	if last {
		return append(data, 1)
	}
	return append(data, 0)
}
