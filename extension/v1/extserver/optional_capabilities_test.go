package extserver

import (
	"testing"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

func TestOptionalCapabilityGrantUnionAndMissingRequiredOnly(t *testing.T) {
	manifest := testManifest(extv1.CapKafkaRead)
	manifest.OptionalCapabilities = []extv1.Capability{extv1.CapEmbeddingInvoke, "future.optional"}
	envMap := fullEnv()
	envMap[EnvCapabilities] = "kafka.read,embedding.invoke,future.optional"
	env, err := loadEnvironment(lookupFrom(envMap), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !env.HasCapability(extv1.CapKafkaRead) || !env.HasCapability(extv1.CapEmbeddingInvoke) {
		t.Fatalf("required/optional grant union lost: %v", env.Capabilities)
	}
	if len(env.MissingCapabilities) != 0 {
		t.Fatalf("optional must not be reported missing: %v", env.MissingCapabilities)
	}
	if len(env.UnknownCapabilities) != 1 || env.UnknownCapabilities[0] != "future.optional" {
		t.Fatalf("unknown optional diagnostics: %v", env.UnknownCapabilities)
	}

	envMap[EnvCapabilities] = ""
	env, err = loadEnvironment(lookupFrom(envMap), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(env.MissingCapabilities) != 1 || env.MissingCapabilities[0] != extv1.CapKafkaRead {
		t.Fatalf("only required capabilities are missing: %v", env.MissingCapabilities)
	}

	delete(envMap, EnvCapabilities)
	env, err = loadEnvironment(lookupFrom(envMap), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !env.HasCapability(extv1.CapKafkaRead) || env.HasCapability(extv1.CapEmbeddingInvoke) {
		t.Fatalf("old Host fallback must not falsely inject optional services: %v", env.Capabilities)
	}
}

func TestOptionalCapabilitiesReachReadyRequestUnion(t *testing.T) {
	manifest := testManifest(extv1.CapKafkaRead)
	manifest.OptionalCapabilities = []extv1.Capability{extv1.CapEmbeddingInvoke, extv1.CapResultRead}
	env := Environment{Version: "1.0.0"}
	var output byteBuffer
	if err := writeHandshake(&output, "127.0.0.1:1", env, manifest); err != nil {
		t.Fatal(err)
	}
	line := output.String()
	for _, want := range []string{"kafka.read", "embedding.invoke", "result.read"} {
		if !contains(line, want) {
			t.Fatalf("READY request union missing %s: %s", want, line)
		}
	}
}

type byteBuffer struct{ data []byte }

func (b *byteBuffer) Write(p []byte) (int, error) { b.data = append(b.data, p...); return len(p), nil }
func (b *byteBuffer) String() string              { return string(b.data) }
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
