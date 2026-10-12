package dtls

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

func CalculateAuthKey(enr, mac string) []byte {
	data := enr + strings.ToUpper(mac)
	hash := sha256.Sum256([]byte(data))
	b64 := base64.StdEncoding.EncodeToString(hash[:6])
	b64 = strings.ReplaceAll(b64, "+", "Z")
	b64 = strings.ReplaceAll(b64, "/", "9")
	b64 = strings.ReplaceAll(b64, "=", "A")
	return []byte(b64)
}

func DerivePSK(enr string) []byte {
	// DerivePSK 从 ENR 派生 DTLS 预共享密钥
	// TUTK SDK 将预共享密钥视为以空字符结尾的 C 字符串；若 SHA256(ENR)
	// 含 0x00 字节，则在该位置截断密钥。
	hash := sha256.Sum256([]byte(enr))
	pskLen := 32
	for i := range 32 {
		if hash[i] == 0x00 {
			pskLen = i
			break
		}
	}

	psk := make([]byte, 32)
	copy(psk[:pskLen], hash[:pskLen])
	return psk
}
