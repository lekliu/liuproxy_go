package data

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	// 定义测试数据
	originalData := []byte("Test data for compression and encryption")

	slice2 := make([]byte, len(originalData))

	// 使用 copy 函数复制切片
	copy(slice2, originalData)

	// 测试加密
	encryptedData := Encrypt(originalData)
	if string(encryptedData) == string(slice2) {
		t.Errorf("Encryption failed: encrypted data should not match the original data")
	}

	// 测试解密
	decryptedData := Decrypt(encryptedData)
	if string(decryptedData) != string(slice2) {
		t.Errorf("Decryption failed: expected %s, got %s", originalData, decryptedData)
	}
}

func TestUpCompressHeader(t *testing.T) {
	// 定义测试数据
	originalData := []byte("Header test data")

	// 测试上行压缩头部数据
	compressedHeaderData := UpCompressHeader(originalData)
	if len(compressedHeaderData) <= len(originalData) {
		t.Errorf("Header compression failed: expected compressed data length to be greater than original")
	}
}

func TestUpDecompressHeader(t *testing.T) {
	// 定义测试数据
	originalData := []byte("Header test data")

	// 模拟上行压缩头部数据
	compressedHeaderData := UpCompressHeader(originalData)

	// 测试上行解压头部数据
	decompressedHeaderData := UpDecompressHeader(compressedHeaderData)
	if string(decompressedHeaderData) != string(originalData) {
		t.Errorf("Header decompression failed: expected %s, got %s", originalData, decompressedHeaderData)
	}
}
