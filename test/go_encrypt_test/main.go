package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"io"
	"log"
)

// deriveKey 从整数密钥派生出32字节的SHA-256哈希密钥
func deriveKey(keyInt int) []byte {
	keyStr := fmt.Sprintf("liuproxy-secure-v2-key-%d", keyInt)
	hash := sha256.Sum256([]byte(keyStr))
	return hash[:]
}

func main() {
	// --- 配置 ---
	keyInt := 125
	plaintext := []byte("Hello, Worker!")
	// --- 结束配置 ---

	fmt.Println("--- Go Encrypt Test for Worker Compatibility ---")

	// 1. 派生密钥
	key := deriveKey(keyInt)
	fmt.Printf("Derived Key (Base64): %s\n", base64.StdEncoding.EncodeToString(key))

	// 2. 创建加密器
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		log.Fatalf("Failed to create AEAD: %v", err)
	}

	// 3. 生成 Nonce
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		log.Fatalf("Failed to generate nonce: %v", err)
	}
	fmt.Printf("Nonce (Base64):       %s\n", base64.StdEncoding.EncodeToString(nonce))

	// 4. 加密
	// Go 的 Seal 方法会自动将 nonce 附加到密文前面
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	fmt.Printf("Ciphertext (Base64):  %s\n\n", base64.StdEncoding.EncodeToString(ciphertext))

	// 5. 为了验证，我们本地解密一次
	decrypted, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		log.Fatalf("Local decryption failed: %v", err)
	}
	fmt.Printf("Local Decryption OK. Result: \"%s\"\n", string(decrypted))
	fmt.Println("\n>>> 请将上面的 Key, Nonce, 和 Ciphertext 的 Base64 值复制到 Worker 的 index.js 文件中进行测试。")
}
