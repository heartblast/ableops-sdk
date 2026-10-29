package testkit_test

import (
	"context"
	"testing"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
	"github.com/heartblast/ableops-sdk/extension/v1/testkit"
)

func TestSemanticFakesPreserveCapabilityIsolation(t *testing.T) {
	embedding := &testkit.FakeEmbedding{}
	results := &testkit.FakeResultStore{}
	host := testkit.NewHost("demo", testkit.WithEmbedding(embedding), testkit.WithResultWrite(results)).Context()
	if _, err := host.RequireEmbedding(); err != nil {
		t.Fatal(err)
	}
	write, err := host.RequireResultWrite()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.RequireResultRead(); extv1.CodeOf(err) != extv1.CodeCapabilityUnavailable {
		t.Fatalf("ungranted result.read resolved: %v", err)
	}
	if _, err := write.ReadResult(context.Background(), extv1.ReadResultRequest{}); extv1.CodeOf(err) != extv1.CodeCapabilityUnavailable {
		t.Fatalf("write-only service allowed read: %v", err)
	}

	both := testkit.NewHost("demo", testkit.WithResultWrite(results), testkit.WithResultRead(results)).Context()
	write, err = both.RequireResultWrite()
	if err != nil {
		t.Fatal(err)
	}
	read, err := both.RequireResultRead()
	if err != nil {
		t.Fatal(err)
	}
	if write != read {
		t.Fatal("read/write grants must resolve the same scoped result store")
	}
}
