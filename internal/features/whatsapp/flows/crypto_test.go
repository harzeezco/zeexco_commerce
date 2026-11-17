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
	"testing"
)

func TestProtocolDecryptsRequestAndEncryptsMetaCompatibleResponse(t *testing.T) {
	privateKey, protocol := testProtocol(t)
	payload := []byte(`{"version":"3.0","action":"ping"}`)
	envelope, aesKey, iv := encryptTestRequest(t, &privateKey.PublicKey, payload)

	decrypted, err := protocol.Decrypt(envelope)
	if err != nil || string(decrypted.Payload) != string(payload) {
		t.Fatalf("Decrypt() payload=%s error=%v", decrypted.Payload, err)
	}
	encryptedResponse, err := protocol.Encrypt(map[string]any{"data": map[string]string{"status": "active"}}, aesKey, iv)
	if err != nil {
		t.Fatal(err)
	}
	response := decryptTestResponse(t, encryptedResponse, aesKey, iv)
	var decoded map[string]map[string]string
	if err := json.Unmarshal(response, &decoded); err != nil || decoded["data"]["status"] != "active" {
		t.Fatalf("response=%s error=%v", response, err)
	}
}

func TestProtocolRejectsInvalidKeyAndEncryptedMaterial(t *testing.T) {
	if _, err := NewProtocol("not-pem", ""); !errors.Is(err, ErrInvalidPrivateKey) {
		t.Fatalf("NewProtocol() error=%v", err)
	}
	_, protocol := testProtocol(t)
	if _, err := protocol.Decrypt(EncryptedRequest{
		EncryptedAESKey: "%%%", EncryptedFlowData: "%%%", InitialVector: "%%%",
	}); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("Decrypt() error=%v", err)
	}
	if _, err := protocol.Decrypt(EncryptedRequest{
		EncryptedAESKey:   base64.StdEncoding.EncodeToString(make([]byte, 256)),
		EncryptedFlowData: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		InitialVector:     base64.StdEncoding.EncodeToString(make([]byte, 16)),
	}); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("Decrypt() RSA error=%v", err)
	}
}

func testProtocol(t *testing.T) (*rsa.PrivateKey, *Protocol) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	protocol, err := NewProtocol(string(pemValue), "")
	if err != nil {
		t.Fatal(err)
	}
	return key, protocol
}

func encryptTestRequest(t *testing.T, publicKey *rsa.PublicKey, payload []byte) (EncryptedRequest, []byte, []byte) {
	t.Helper()
	aesKey := make([]byte, flowAESKeyBytes)
	iv := make([]byte, flowIVBytes)
	if _, err := rand.Read(aesKey); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(iv); err != nil {
		t.Fatal(err)
	}
	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, aesKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(aesKey)
	gcm, _ := cipher.NewGCMWithNonceSize(block, flowIVBytes)
	ciphertext := gcm.Seal(nil, iv, payload, nil)
	return EncryptedRequest{
		EncryptedAESKey:   base64.StdEncoding.EncodeToString(encryptedKey),
		EncryptedFlowData: base64.StdEncoding.EncodeToString(ciphertext),
		InitialVector:     base64.StdEncoding.EncodeToString(iv),
	}, aesKey, iv
}

func decryptTestResponse(t *testing.T, encoded string, aesKey, requestIV []byte) []byte {
	t.Helper()
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	flipped := make([]byte, len(requestIV))
	for index, value := range requestIV {
		flipped[index] = ^value
	}
	block, _ := aes.NewCipher(aesKey)
	gcm, _ := cipher.NewGCMWithNonceSize(block, flowIVBytes)
	plaintext, err := gcm.Open(nil, flipped, ciphertext, nil)
	if err != nil {
		t.Fatal(err)
	}
	return plaintext
}
