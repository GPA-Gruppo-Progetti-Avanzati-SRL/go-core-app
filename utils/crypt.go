package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

// Encrypt cifra plaintext con AES-256-GCM, chiave = SHA-256(keyStr), e ritorna nonce||ciphertext
// in BINARIO. Decrypt si aspetta la stessa sequenza codificata in esadecimale: è il formato del
// token di go-core-auth (apiauth fa hex.EncodeToString del risultato), e il frontdoor che lo
// decritta dipende da questa derivazione della chiave — non è un KDF, ma cambiarla romperebbe i
// token già emessi e chi li legge.
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

// Decrypt decritta un messaggio esadecimale usando keyStr (l'AppID) come chiave: l'inverso di
// hex.EncodeToString(Encrypt(...)). Il nonce è atteso all'inizio del ciphertext decodificato.
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
