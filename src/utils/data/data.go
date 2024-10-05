package data

// DownCompress compresses and encrypts the data for "downstream"
func DownCompress(data []byte) []byte {
	return encrypt(data)
}

// DownDecompress decrypts and decompresses the data for "downstream"
func DownDecompress(data []byte) []byte {
	data = decrypt(data)
	return data
}

// UpCompress compresses and encrypts the data for "upstream"
func UpCompress(data []byte) []byte {
	return encrypt(data)
}

// UpDecompress decrypts and decompresses the data for "upstream"
func UpDecompress(data []byte) []byte {
	data = decrypt(data)
	// Uncomment this line if zlib decompression is needed
	return data
}

// UpCompressHeader compresses the header and encrypts the data for "upstream"
func UpCompressHeader(data []byte) []byte {
	header := []byte("GET / HTTP/1.1\r\nHost: 43.128.59.144:443\r\nConnection: keep-alive\r\nUpgrade-Insecure-Requests: 1\r\nUser-Agent: Mozilla/5.0 (Windows NT 6.1; WOW64)\r\nAccept: text/html\r\n\r\n")
	data = encrypt(data)
	return append(header, data...)
}

// UpDecompressHeader decrypts the header and decompresses the data for "upstream"
func UpDecompressHeader(data []byte) []byte {
	headerLen := len("GET / HTTP/1.1\r\nHost: 43.128.59.144:443\r\nConnection: keep-alive\r\nUpgrade-Insecure-Requests: 1\r\nUser-Agent: Mozilla/5.0 (Windows NT 6.1; WOW64)\r\nAccept: text/html\r\n\r\n")
	data = decrypt(data[headerLen:])
	return data
}

func SocksCompress(data []byte) []byte {
	data = encrypt(data)
	return data
}

func SocksDecompress(data []byte) []byte {
	data = decrypt(data)
	return data
}

// Encrypt encrypts the data by adding 125 to each byte and wrapping around using modulo
func encrypt(data []byte) []byte {
	for i := range data {
		data[i] = byte((int(data[i]) + 125) % 256) // 显式转换为 int 进行运算后再转换回 byte
	}
	return data
}

// Decrypt decrypts the data by subtracting 125 from each byte and wrapping around using modulo
func decrypt(data []byte) []byte {
	for i := range data {
		data[i] = byte((int(data[i]) + 256 - 125) % 256)
	}
	return data
}
