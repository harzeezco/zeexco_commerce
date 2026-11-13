package flows

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestMerchantRegistrationFlowJSONContract(t *testing.T) {
	raw, err := os.ReadFile("../../../../docs/flows/merchant-registration.flow.json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Version        string `json:"version"`
		DataAPIVersion string `json:"data_api_version"`
		Screens        []struct {
			ID       string `json:"id"`
			Terminal bool   `json:"terminal"`
			Layout   struct {
				Children []json.RawMessage `json:"children"`
			} `json:"layout"`
		} `json:"screens"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("invalid Flow JSON: %v", err)
	}
	if document.Version != "7.3" || document.DataAPIVersion != DataAPIVersion ||
		len(document.Screens) != 1 || document.Screens[0].ID != RegistrationScreen ||
		!document.Screens[0].Terminal {
		t.Fatalf("unexpected Flow document contract: %#v", document)
	}
	serialized := string(raw)
	for _, required := range []string{
		`"name": "first_name"`, `"name": "last_name"`,
		`"name": "business_name"`, `"name": "email"`,
		`"name": "data_exchange"`, `"label": "Create my business"`,
	} {
		if !strings.Contains(serialized, required) {
			t.Fatalf("Flow JSON missing %s", required)
		}
	}
	for _, forbidden := range []string{
		`"name": "password"`, `"name": "confirm_password"`,
		`"name": "whatsapp_number"`, `"name": "wa_id"`,
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("Flow JSON contains forbidden field %s", forbidden)
		}
	}
}
