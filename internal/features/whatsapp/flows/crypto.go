package flows

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

const (
	flowAESKeyBytes = 16
	flowIVBytes     = 16
	minimumRSABits  = 2048
)

var (
	ErrInvalidPrivateKey = errors.New("invalid WhatsApp Flow private key")
	ErrInvalidEnvelope   = errors.New("invalid WhatsApp Flow encrypted envelope")
	ErrDecrypt           = errors.New("cannot decrypt WhatsApp Flow request")
	ErrEncrypt           = errors.New("cannot encrypt WhatsApp Flow response")
)

type EncryptedRequest struct {
	EncryptedAESKey   string `json:"encrypted_aes_key"`
	EncryptedFlowData string `json:"encrypted_flow_data"`
	InitialVector     string `json:"initial_vector"`
}

type DecryptedRequest struct {
	Payload       []byte
	AESKey        []byte
	InitialVector []byte
}

type Protocol struct {
	privateKey *rsa.PrivateKey
}

func NewProtocol(privateKeyPEM, passphrase string) (*Protocol, error) {
	key, err := parseRSAPrivateKey(privateKeyPEM, passphrase)
	if err != nil {
		return nil, err
	}
	return &Protocol{privateKey: key}, nil
}

func (p *Protocol) Decrypt(request EncryptedRequest) (DecryptedRequest, error) {
	if p == nil || p.privateKey == nil || request.EncryptedAESKey == "" ||
		request.EncryptedFlowData == "" || request.InitialVector == "" {
		return DecryptedRequest{}, ErrInvalidEnvelope
	}
	encryptedKey, err := decodeBase64(request.EncryptedAESKey)
	if err != nil {
		return DecryptedRequest{}, ErrInvalidEnvelope
	}
	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, p.privateKey, encryptedKey, nil)
	if err != nil || len(aesKey) != flowAESKeyBytes {
		return DecryptedRequest{}, ErrDecrypt
	}
	iv, err := decodeBase64(request.InitialVector)
	if err != nil || len(iv) != flowIVBytes {
		return DecryptedRequest{}, ErrInvalidEnvelope
	}
	ciphertext, err := decodeBase64(request.EncryptedFlowData)
	if err != nil || len(ciphertext) < 16 {
		return DecryptedRequest{}, ErrInvalidEnvelope
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return DecryptedRequest{}, ErrDecrypt
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, flowIVBytes)
	if err != nil {
		return DecryptedRequest{}, ErrDecrypt
	}
	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return DecryptedRequest{}, ErrDecrypt
	}
	if !json.Valid(plaintext) {
		return DecryptedRequest{}, ErrInvalidEnvelope
	}
	return DecryptedRequest{
		Payload: plaintext, AESKey: append([]byte(nil), aesKey...),
		InitialVector: append([]byte(nil), iv...),
	}, nil
}

func (p *Protocol) Encrypt(response any, aesKey, requestIV []byte) (string, error) {
	if len(aesKey) != flowAESKeyBytes || len(requestIV) != flowIVBytes {
		return "", ErrEncrypt
	}
	plaintext, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("%w: marshal response", ErrEncrypt)
	}
	flippedIV := make([]byte, len(requestIV))
	for index, value := range requestIV {
		flippedIV[index] = ^value
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", ErrEncrypt
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, flowIVBytes)
	if err != nil {
		return "", ErrEncrypt
	}
	ciphertext := gcm.Seal(nil, flippedIV, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func parseRSAPrivateKey(value, passphrase string) (*rsa.PrivateKey, error) {
	block, rest := pem.Decode([]byte(strings.TrimSpace(value)))
	if block == nil || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, ErrInvalidPrivateKey
	}
	der := block.Bytes
	if x509.IsEncryptedPEMBlock(block) { // Meta's reference generator emits encrypted PKCS#1 PEM.
		if passphrase == "" {
			return nil, ErrInvalidPrivateKey
		}
		decrypted, err := x509.DecryptPEMBlock(block, []byte(passphrase)) //nolint:staticcheck // required for Meta-compatible legacy encrypted PEM
		if err != nil {
			return nil, ErrInvalidPrivateKey
		}
		der = decrypted
	} else if strings.Contains(block.Type, "ENCRYPTED") {
		return nil, ErrInvalidPrivateKey
	}

	var key *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		key = parsed
	} else if parsed, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		var ok bool
		key, ok = parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, ErrInvalidPrivateKey
		}
	} else {
		return nil, ErrInvalidPrivateKey
	}
	if key.N.BitLen() < minimumRSABits || key.Validate() != nil {
		return nil, ErrInvalidPrivateKey
	}
	return key, nil
}

func decodeBase64(value string) ([]byte, error) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}
