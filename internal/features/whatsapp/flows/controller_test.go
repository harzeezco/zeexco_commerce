package flows

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/onboarding"
)

const testAppSecret = "test-meta-app-secret"

func TestEncryptedPingReturnsActiveWithoutRegistrationWrite(t *testing.T) {
	privateKey, protocol := testProtocol(t)
	registrar := &fakeRegistrar{}
	controller, err := NewController(protocol, NewActionHandler(registrar), testAppSecret, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	envelope, aesKey, iv := encryptTestRequest(t, &privateKey.PublicKey, []byte(`{"version":"3.0","action":"ping"}`))
	body, _ := json.Marshal(envelope)
	request := signedFlowRequest(body)
	response := httptest.NewRecorder()

	controller.Exchange(response, request)

	if response.Code != http.StatusOK || registrar.calls != 0 || response.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("status=%d calls=%d headers=%v body=%s", response.Code, registrar.calls, response.Header(), response.Body.String())
	}
	decrypted := decryptTestResponse(t, response.Body.String(), aesKey, iv)
	if !bytes.Contains(decrypted, []byte(`"status":"active"`)) {
		t.Fatalf("decrypted response=%s", decrypted)
	}
}

func TestFlowControllerRejectsBadSignatureMalformedEncryptionAndUnknownAction(t *testing.T) {
	privateKey, protocol := testProtocol(t)
	controller, _ := NewController(protocol, NewActionHandler(nil), testAppSecret, log.New(io.Discard, "", 0))

	badSignature := httptest.NewRecorder()
	controller.Exchange(badSignature, httptest.NewRequest(http.MethodPost, "/api/v1/whatsapp/flows", bytes.NewReader([]byte(`{}`))))
	if badSignature.Code != 432 {
		t.Fatalf("signature status=%d", badSignature.Code)
	}

	malformedBody, _ := json.Marshal(EncryptedRequest{
		EncryptedAESKey: "bad", EncryptedFlowData: "bad", InitialVector: "bad",
	})
	malformed := httptest.NewRecorder()
	controller.Exchange(malformed, signedFlowRequest(malformedBody))
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed status=%d", malformed.Code)
	}

	envelope, aesKey, iv := encryptTestRequest(t, &privateKey.PublicKey, []byte(`{"version":"3.0","action":"unknown"}`))
	unknownBody, _ := json.Marshal(envelope)
	unknown := httptest.NewRecorder()
	controller.Exchange(unknown, signedFlowRequest(unknownBody))
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown status=%d", unknown.Code)
	}
	if !bytes.Contains(decryptTestResponse(t, unknown.Body.String(), aesKey, iv), []byte("Unsupported Flow action")) {
		t.Fatal("unknown action did not return an encrypted deterministic error")
	}
}

func TestFlowControllerRejectsEnvelopeWithUnknownFields(t *testing.T) {
	_, protocol := testProtocol(t)
	controller, _ := NewController(protocol, NewActionHandler(nil), testAppSecret, log.New(io.Discard, "", 0))
	body := []byte(`{"encrypted_aes_key":"a","encrypted_flow_data":"b","initial_vector":"c","unexpected":true}`)
	response := httptest.NewRecorder()

	controller.Exchange(response, signedFlowRequest(body))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

func signedFlowRequest(body []byte) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/whatsapp/flows", bytes.NewReader(body))
	mac := hmac.New(sha256.New, []byte(testAppSecret))
	_, _ = mac.Write(body)
	request.Header.Set(flowSignatureHeader, flowSignaturePrefix+hex.EncodeToString(mac.Sum(nil)))
	return request
}

var _ Registrar = (*noWriteRegistrar)(nil)

type noWriteRegistrar struct{}

func (*noWriteRegistrar) Complete(context.Context, string, onboarding.RegisterCommand) (onboarding.Completion, error) {
	panic("health check must not register")
}
