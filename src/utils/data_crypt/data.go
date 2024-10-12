package data_crypt

var Header = []byte("GET / HTTP/1.1\r\nHost: 43.128.59.144:443\r\nConnection: keep-alive\r\nUser-Agent: Mozilla/5.0 (Windows NT 6.1)\r\nAccept: text/html\r\n")

// DownCompress compresses and encrypts the data_crypt for "downstream"
func DownCompress(data []byte, cryptno int) []byte {
	return encrypt(data, cryptno)
}

// DownDecompress decrypts and decompresses the data_crypt for "downstream"
func DownDecompress(data []byte, cryptno int) []byte {
	data = decrypt(data, cryptno)
	return data
}

// UpCompress compresses and encrypts the data_crypt for "upstream"
func UpCompress(data []byte, cryptno int) []byte {
	return encrypt(data, cryptno)
}

// UpDecompress decrypts and decompresses the data_crypt for "upstream"
func UpDecompress(data []byte, cryptno int) []byte {
	data = decrypt(data, cryptno)
	// Uncomment this line if zlib decompression is needed
	return data
}

// UpCompressHeader compresses the header and encrypts the data_crypt for "upstream"
func UpCompressHeader(data []byte, cryptno int) []byte {
	data = encrypt(data, cryptno)
	return append(Header, data...)
}

// UpDecompressHeader decrypts the header and decompresses the data_crypt for "upstream"
func UpDecompressHeader(data []byte, cryptno int) []byte {
	headerLen := len(Header)
	if len(data) <= headerLen {
		return []byte{}
	}
	data = decrypt(data[headerLen:], cryptno)
	return data
}

func SocksCompress(data []byte, cryptno int) []byte {
	data = encrypt(data, cryptno)
	return data
}

func SocksDecompress(data []byte, cryptno int) []byte {
	data = decrypt(data, cryptno)
	return data
}

// Encrypt encrypts the data_crypt by adding 125 to each byte and wrapping around using modulo
func encrypt(data []byte, cryptno int) []byte {
	for i := range data {
		data[i] = byte((int(data[i]) + cryptno) % 256) // 显式转换为 int 进行运算后再转换回 byte
	}
	return data
}

// Decrypt decrypts the data_crypt by subtracting 125 from each byte and wrapping around using modulo
func decrypt(data []byte, cryptno int) []byte {
	for i := range data {
		data[i] = byte((int(data[i]) + 256 - cryptno) % 256)
	}
	return data
}
