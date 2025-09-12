package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
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
	plaintext := []byte("Hello from AES-GCM!")
	// --- 结束配置 ---

	fmt.Println("--- Go AES-256-GCM Encrypt Test for Worker Compatibility ---")

	// 1. 派生密钥
	key := deriveKey(keyInt)
	fmt.Printf("Derived Key (Base64): %s\n", base64.StdEncoding.EncodeToString(key))

	// 2. 创建 AES-GCM 加密器
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Fatalf("Failed to create cipher block: %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		log.Fatalf("Failed to create GCM: %v", err)
	}

	// 3. 生成 Nonce (IV)
	nonce := make([]byte, aead.NonceSize()) // 通常是 12 字节
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		log.Fatalf("Failed to generate nonce: %v", err)
	}
	fmt.Printf("Nonce (Base64, %d bytes): %s\n", len(nonce), base64.StdEncoding.EncodeToString(nonce))

	// 4. 加密
	// Go 的 Seal 会将密文附加在 nonce 之后返回, 形成 [nonce][ciphertext+tag] 的结构
	ciphertextWithNonce := aead.Seal(nonce, nonce, plaintext, nil)

	fmt.Printf("Ciphertext with Nonce (Base64): %s\n\n", base64.StdEncoding.EncodeToString(ciphertextWithNonce))

	// 5. 本地解密验证
	decrypted, err := aead.Open(nil, nonce, ciphertextWithNonce[len(nonce):], nil)
	if err != nil {
		log.Fatalf("Local decryption failed: %v", err)
	}
	fmt.Printf("Local Decryption OK. Result: \"%s\"\n", string(decrypted))
	fmt.Println("\n>>> 请将上面的 Key 和 Ciphertext with Nonce 的 Base64 值复制到 Worker 的 crypto.js 文件中进行测试。")
}
