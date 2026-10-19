package extensionv1

import (
	"encoding/json"
	"testing"
)

func TestReliabilityManifestAdditionsAreOptional(t *testing.T) {
	m := mustParse(t, validManifestYAML)
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	route := wire["routes"].([]any)[0].(map[string]any)
	if _, ok := route["methods"]; ok {
		t.Fatal("optional methods changed old wire")
	}
	m.Routes[0].Methods = []string{"GET", "POST"}
	m.Config.Schema = []ConfigField{{Key: "credential", Type: ConfigTypeSecretRef}, {Key: "mode", Type: ConfigTypeSelect, Default: "safe", Options: []SelectOption{{Value: "safe", Label: "Safe"}}}}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if !m.Routes[0].AllowsMethod("HEAD") || m.Routes[0].AllowsMethod("DELETE") {
		t.Fatal("method boundary")
	}
	sanitized := m.Sanitized()
	sanitized.Routes[0].Methods[0] = "DELETE"
	sanitized.Config.Schema[1].Options[0].Value = "unsafe"
	if m.Routes[0].Methods[0] != "GET" || m.Config.Schema[1].Options[0].Value != "safe" {
		t.Fatal("sanitized aliases mutable declarations")
	}
}
func TestReliabilityManifestRejectsUnsafeDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"method", func(m *Manifest) { m.Routes[0].Methods = []string{"TRACE"} }},
		{"method duplicate", func(m *Manifest) { m.Routes[0].Methods = []string{"GET", "GET"} }},
		{"secret default", func(m *Manifest) {
			m.Config.Schema = []ConfigField{{Key: "credential", Type: ConfigTypeSecretRef, Default: "raw-secret"}}
		}},
		{"secret flag", func(m *Manifest) {
			m.Config.Schema = []ConfigField{{Key: "credential", Type: ConfigTypeSecretRef, Secret: true}}
		}},
		{"option duplicate", func(m *Manifest) {
			m.Config.Schema = []ConfigField{{Key: "mode", Type: ConfigTypeSelect, Options: []SelectOption{{Value: "x"}, {Value: "x"}}}}
		}},
		{"option default", func(m *Manifest) {
			m.Config.Schema = []ConfigField{{Key: "mode", Type: ConfigTypeSelect, Default: "outside", Options: []SelectOption{{Value: "x"}}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := mustParse(t, validManifestYAML)
			tc.mutate(&m)
			if m.Validate() == nil {
				t.Fatal("unsafe declaration accepted")
			}
		})
	}
}

func TestSecretSelectOptionsAreRejectedAndRedacted(t *testing.T) {
	m := mustParse(t, validManifestYAML)
	m.Config.Schema = []ConfigField{{Key: "password", Type: ConfigTypeSelect, Options: []SelectOption{{Value: "credential-bytes"}}}}
	if m.Validate() == nil {
		t.Fatal("credential choice declaration accepted")
	}
	if len(m.Sanitized().Config.Schema[0].Options) != 0 {
		t.Fatal("credential choices exposed")
	}
}
