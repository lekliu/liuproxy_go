// 修改点 1: 移除了所有压缩/解压逻辑 **********
// Modification Point 1: Removed all compression/decompression logic **********
// 原始行号: (多处 / Multiple Lines)
package crypt

var Header = []byte("GET / HTTP/1.1\r\nHost: 43.128.59.18:443\r\nConnection: keep-alive\r\nUser-Agent: Mozilla/5.0 (Windows NT 6.1)\r\nAccept: text/html\r\n")

// Encrypt 只执行加密
func Encrypt(data []byte, key int) []byte {
	return transform(data, key)
}

// Decrypt 只执行解密
func Decrypt(data []byte, key int) []byte {
	return transform(data, 256-key)
}

// transform 在数据的副本上执行位运算
func transform(data []byte, key int) []byte {
	result := make([]byte, len(data))
	for i := range data {
		result[i] = byte((int(data[i]) + key) % 256)
	}
	return result
}

// EncryptWithHeader 先加密，再加头
func EncryptWithHeader(data []byte, key int) []byte {
	encryptedData := Encrypt(data, key)
	return append(Header, encryptedData...)
}

// DecryptWithHeader 先去头，再解密
func DecryptWithHeader(data []byte, key int) []byte {
	headerLen := len(Header)
	if len(data) <= headerLen {
		return []byte{}
	}
	return Decrypt(data[headerLen:], key)
}
