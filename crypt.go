package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

func Encrypt(plaintext []byte, keyStr string) ([]byte, error) {
	// Crea una chiave di 32 byte dall'AppID usando SHA-256
	hash := sha256.Sum256([]byte(keyStr))
	key := hash[:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Concatena il nonce al ciphertext.
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt decritta un messaggio esadecimale usando l'AppID come chiave.
// Il nonce è atteso all'inizio del ciphertext decodificato da hex.
func Decrypt(ciphertextHex string, keyStr string) ([]byte, error) {
	ciphertext, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256([]byte(keyStr))
	key := hash[:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
