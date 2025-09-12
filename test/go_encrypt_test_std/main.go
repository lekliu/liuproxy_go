package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"

	"golang.org/x/crypto/chacha20poly1305" // 仍然是这个包
)

func deriveKey(keyInt int) []byte {
	keyStr := fmt.Sprintf("liuproxy-secure-v2-key-%d", keyInt)
	hash := sha256.Sum256([]byte(keyStr))
	return hash[:]
}

func main() {
	fmt.Println("--- Go STANDARD ChaCha20-Poly1305 Encrypt Test ---")
	keyInt := 125
	plaintext := []byte("Hello from Standard ChaCha20!")

	key := deriveKey(keyInt)
	fmt.Printf("Derived Key (Base64): %s\n", base64.StdEncoding.EncodeToString(key))

	// --- 关键改动: 使用 New 而不是 NewX ---
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		log.Fatalf("Failed to create standard AEAD: %v", err)
	}

	// Nonce 大小现在是 12 字节
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		log.Fatalf("Failed to generate 12-byte nonce: %v", err)
	}
	fmt.Printf("Nonce (Base64, 12 bytes): %s\n", base64.StdEncoding.EncodeToString(nonce))

	// 加密逻辑不变
	// 注意：Go 的 Seal 会将密文附加在 nonce 之后返回。所以结果是 [nonce][ciphertext]
	ciphertextWithNonce := aead.Seal(nonce, nonce, plaintext, nil)
	// 我们需要分离它们，以匹配 JS 库的 API
	ciphertextOnly := ciphertextWithNonce[len(nonce):]

	fmt.Printf("Ciphertext ONLY (Base64): %s\n\n", base64.StdEncoding.EncodeToString(ciphertextOnly))

	fmt.Println(">>> 请将 Key, Nonce, 和 Ciphertext ONLY 的值复制到 Worker 中测试。")
}
