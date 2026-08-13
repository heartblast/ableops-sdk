package extensionv1

// Capability 상수·API 버전 판정 테스트.

import "testing"

func TestKnownCapability(t *testing.T) {
	if len(AllCapabilities()) != 6 {
		t.Fatalf("capability 는 6종이어야 한다: %d종", len(AllCapabilities()))
	}
	for _, c := range AllCapabilities() {
		if !KnownCapability(c) {
			t.Errorf("KnownCapability(%q) = false", c)
		}
	}
	// 오타·미지원 값은 반드시 거부한다(런타임 nil 역참조 대신 설치 시점에 걸린다).
	for _, c := range []Capability{"", "kafka.write", "kafka_read", "KAFKA.READ", "workflow.approve", "secret.read"} {
		if KnownCapability(c) {
			t.Errorf("KnownCapability(%q) = true (거부해야 함)", c)
		}
	}
}

func TestCapability_상수값(t *testing.T) {
	want := map[Capability]string{
		CapKafkaRead:      "kafka.read",
		CapClusterRead:    "cluster.read",
		CapWorkflowSubmit: "workflow.submit",
		CapAuditWrite:     "audit.write",
		CapConfigRead:     "config.read",
		CapSecretRef:      "secret.ref",
	}
	for c, s := range want {
		if string(c) != s {
			t.Errorf("capability 상수값이 바뀌었다: %q != %q (Manifest 하위호환 파괴)", string(c), s)
		}
	}
}

func TestSupportsAPIVersion(t *testing.T) {
	if APIVersion != "ableops.io/extension/v1" {
		t.Fatalf("APIVersion 상수가 바뀌었다: %q", APIVersion)
	}
	if !SupportsAPIVersion(APIVersion) || !SupportsAPIVersion("  "+APIVersion+"  ") {
		t.Error("정상 apiVersion 을 거부했다")
	}
	for _, v := range []string{"", "  ", "ableops.io/extension/v2", "ableops.io/extension", "v1", "ABLEOPS.IO/EXTENSION/V1"} {
		if SupportsAPIVersion(v) {
			t.Errorf("SupportsAPIVersion(%q) = true (거부해야 함)", v)
		}
	}
}
