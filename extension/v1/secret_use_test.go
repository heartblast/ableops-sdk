package extensionv1

import (
	"net/http"
	"testing"
)

func TestSecretEgressOrigin(t *testing.T) {
	ok := map[string]string{
		"https://API.Example.com/v1/chat?x=1": "https://api.example.com",
		"https://api.example.com:443/":        "https://api.example.com",
		"http://127.0.0.1:8080/v1":            "http://127.0.0.1:8080",
		"http://[::1]:80/":                    "http://[::1]",
	}
	for in, want := range ok {
		if got, err := SecretEgressOrigin(in); err != nil || got != want {
			t.Errorf("%s = %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "/relative", "ftp://x.example.com/", "https://u:p@x.example.com/", "https://x.example.com/#f", "mailto:a@b.c", "https:///nohost"} {
		if _, err := SecretEgressOrigin(bad); CodeOf(err) != CodeInvalidArgument {
			t.Errorf("%q must be rejected: %v", bad, err)
		}
	}
}

func TestSanitizeSecretEgressHeader(t *testing.T) {
	in := http.Header{}
	for _, k := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Host", "Connection", "Transfer-Encoding", "X-Ableops-User-Id", "X-Api-Key"} {
		in.Set(k, "v")
	}
	in.Set("Content-Type", "application/json")
	in.Set("Anthropic-Version", "2023-06-01")
	out := SanitizeSecretEgressHeader(in, "x-api-key")
	if len(out) != 2 || out.Get("Content-Type") != "application/json" || out.Get("Anthropic-Version") == "" {
		t.Fatalf("sanitized: %v", out)
	}
	out.Set("Content-Type", "changed")
	if in.Get("Content-Type") != "application/json" {
		t.Fatal("input mutated")
	}
}
