package backend

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func LoadSecretKey(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		if len(b) != 32 {
			return nil, fmt.Errorf("secret.keyの長さが不正です")
		}
		return b, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	b = make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		return nil, err
	}
	return b, nil
}
func Encrypt(key, plain []byte) ([]byte, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, plain, nil), nil
}
func Decrypt(key, data []byte) ([]byte, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	n := g.NonceSize()
	if len(data) < n {
		return nil, fmt.Errorf("暗号化値が不正です")
	}
	return g.Open(nil, data[:n], data[n:], nil)
}
