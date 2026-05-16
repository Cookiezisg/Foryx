package crypto

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const v1Prefix = "v1:"

// AESGCMEncryptor implements domain/crypto.Encryptor with AES-256-GCM; wire = "v1:"+base64(nonce||ct||tag).
//
// AESGCMEncryptor 用 AES-256-GCM 实现 domain/crypto.Encryptor，线上格式 "v1:"+base64(nonce||密文||tag)。
type AESGCMEncryptor struct {
	gcm cipher.AEAD
}

// NewAESGCMEncryptor builds an encryptor from a 32-byte master key (use DeriveKey).
//
// NewAESGCMEncryptor 用 32 字节主密钥构造 encryptor（搭配 DeriveKey）。
func NewAESGCMEncryptor(masterKey []byte) (*AESGCMEncryptor, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("aesgcm: master key must be 32 bytes, got %d", len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("aesgcm: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aesgcm: new GCM: %w", err)
	}
	return &AESGCMEncryptor{gcm: gcm}, nil
}

// DeriveKey stretches a fingerprint to 32-byte AES key; changing the salt invalidates all v1 ciphertexts.
//
// DeriveKey 把指纹拉伸成 32 字节 AES 密钥；改 salt 会让所有 v1 密文失效。
func DeriveKey(fingerprint string) []byte {
	const salt = "forgify:aesgcm:v1:1ZOI95qH2X" // do not change / 勿改
	h := sha256.Sum256([]byte(salt + "|" + fingerprint))
	return h[:]
}

// Encrypt seals plaintext with a fresh random nonce per call (IND-CPA).
//
// Encrypt 每次调用用全新随机 nonce 加密（IND-CPA 安全）。
func (e *AESGCMEncryptor) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("aesgcm: generate nonce: %w", err)
	}
	sealed := e.gcm.Seal(nonce, nonce, plaintext, nil)
	encoded := base64.StdEncoding.EncodeToString(sealed)
	return []byte(v1Prefix + encoded), nil
}

// Decrypt opens a v1 ciphertext; non-v1 input returns ErrUnsupportedVersion.
//
// Decrypt 解开 v1 密文；非 v1 输入返回 ErrUnsupportedVersion。
func (e *AESGCMEncryptor) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	if !bytes.HasPrefix(ciphertext, []byte(v1Prefix)) {
		return nil, ErrUnsupportedVersion
	}
	encoded := ciphertext[len(v1Prefix):]
	sealed, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, fmt.Errorf("aesgcm: base64 decode: %w", err)
	}
	nonceSize := e.gcm.NonceSize()
	if len(sealed) < nonceSize {
		return nil, fmt.Errorf("aesgcm: ciphertext too short (%d < %d)", len(sealed), nonceSize)
	}
	plaintext, err := e.gcm.Open(nil, sealed[:nonceSize], sealed[nonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("aesgcm: open: %w", err)
	}
	return plaintext, nil
}

// ErrUnsupportedVersion signals Decrypt got a ciphertext version it can't handle.
//
// ErrUnsupportedVersion 表示 Decrypt 遇到不支持的密文版本。
var ErrUnsupportedVersion = errors.New("aesgcm: unsupported ciphertext version")
