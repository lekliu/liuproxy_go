// --- START OF NEW FILE internal/core/securecrypt/cipher_test.go ---
package securecrypt

import (
	"bytes"
	"testing"
)

func TestCipher_EncryptDecrypt_Success(t *testing.T) {
	// 1. 准备
	key := 125
	plaintext := []byte("this is a secret message that needs to be encrypted")

	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher() failed: %v", err)
	}

	// 2. 加密
	ciphertext, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() failed: %v", err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Fatal("Encrypt() returned plaintext, encryption failed.")
	}

	// 3. 解密
	decryptedText, err := cipher.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() failed: %v", err)
	}

	// 4. 验证
	if !bytes.Equal(plaintext, decryptedText) {
		t.Errorf("Decrypted text does not match original plaintext.")
		t.Errorf("Original:  %s", string(plaintext))
		t.Errorf("Decrypted: %s", string(decryptedText))
	}
}

func TestCipher_Decrypt_WrongKey(t *testing.T) {
	// 1. 准备
	key1 := 125
	key2 := 126 // 错误的密钥
	plaintext := []byte("another secret message")

	cipher1, _ := NewCipher(key1)
	cipher2, _ := NewCipher(key2)

	// 2. 用 key1 加密
	ciphertext, _ := cipher1.Encrypt(plaintext)

	// 3. 用 key2 解密
	_, err := cipher2.Decrypt(ciphertext)

	// 4. 验证
	if err == nil {
		t.Fatal("Decrypt() should have failed with the wrong key, but it succeeded.")
	} else {
		// 打印错误以确认是认证错误，这是预期的
		t.Logf("Successfully caught expected error on wrong key: %v", err)
	}
}

func TestCipher_Decrypt_TamperedData(t *testing.T) {
	// 1. 准备
	key := 125
	plaintext := []byte("message that will be tampered")
	cipher, _ := NewCipher(key)

	// 2. 加密
	ciphertext, _ := cipher.Encrypt(plaintext)

	// 3. 篡改密文 (翻转密文中间的一个 bit)
	if len(ciphertext) > 20 { // 确保密文足够长
		ciphertext[len(ciphertext)/2] ^= 0x01 // XOR a bit in the middle
	} else {
		t.Skip("Ciphertext too short for tampering test, skipping.")
	}

	// 4. 解密篡改后的数据
	_, err := cipher.Decrypt(ciphertext)

	// 5. 验证
	if err == nil {
		t.Fatal("Decrypt() should have failed with tampered data, but it succeeded.")
	} else {
		// 打印错误以确认是认证错误，这是预期的
		t.Logf("Successfully caught expected error on tampered data: %v", err)
	}
}
