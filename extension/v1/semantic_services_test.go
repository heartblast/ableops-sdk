package extensionv1

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"reflect"
	"strings"
	"testing"
)

func validEmbeddingRequest() EmbeddingRequest {
	return EmbeddingRequest{
		Inputs: []EmbeddingInput{{Index: 0, Text: "query: kafka lag"}},
		Model:  EmbeddingModelIdentity{ModelID: "e5", ModelRevision: "rev1", TokenizerRevision: "tok1", Dimensions: 2, PoolingNormalization: "l2", PreprocessingVersion: "query-v1"},
	}
}

func TestEmbeddingValidationRejectsDimensionOrderAndInvalidVectors(t *testing.T) {
	request := validEmbeddingRequest()
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := request
	bad.Inputs = []EmbeddingInput{{Index: 1, Text: "x"}}
	if CodeOf(bad.Validate()) != CodeInvalidArgument {
		t.Fatal("out-of-order input accepted")
	}
	response := EmbeddingResponse{Model: request.Model, Vectors: []EmbeddingVector{{Index: 0, Values: []float32{0.6, 0.8}}}}
	if err := response.ValidateFor(request); err != nil {
		t.Fatal(err)
	}
	response.Vectors[0].Values[0] = float32(math.NaN())
	if CodeOf(response.ValidateFor(request)) != CodeIncompatible {
		t.Fatal("NaN accepted")
	}
	response.Vectors[0].Values = []float32{0, 0}
	if CodeOf(response.ValidateFor(request)) != CodeIncompatible {
		t.Fatal("zero vector accepted")
	}
}

func TestResultValidationRequiresExactLengthHashAndBoundedSelectors(t *testing.T) {
	data := []byte("hello")
	sum := sha256.Sum256(data)
	request := PutResultRequest{Data: data, Length: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), MIMEType: "text/plain", Source: ResultSourceIdentity{ExtensionID: "ai-ext", ServerID: "kafka", ToolName: "lag", SchemaHash: strings.Repeat("a", 64)}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := request
	bad.Length++
	if CodeOf(bad.Validate()) != CodeInvalidArgument {
		t.Fatal("length mismatch accepted")
	}
	bad = request
	bad.SHA256 = strings.Repeat("0", 64)
	if CodeOf(bad.Validate()) != CodeInvalidArgument {
		t.Fatal("hash mismatch accepted")
	}
	page := ReadResultPageRequest{Reference: ResultReference{Ref: "opaque-result-ref"}, Limit: 1, Fields: []string{"/rows/0"}}
	if err := page.Validate(); err != nil {
		t.Fatal(err)
	}
	page.Fields = []string{"not-a-pointer"}
	if CodeOf(page.Validate()) != CodeInvalidArgument {
		t.Fatal("unbounded selector accepted")
	}
	page.Fields = []string{"/bad~2escape"}
	if CodeOf(page.Validate()) != CodeInvalidArgument {
		t.Fatal("invalid RFC 6901 escape accepted")
	}
}
func TestResultRequestsCannotCarryAuthorityOrStorageRouting(t *testing.T) {
	requests := []any{
		PutResultRequest{}, ReadResultRequest{}, ReadResultPageRequest{}, GetResultInfoRequest{},
	}
	forbidden := map[string]bool{
		"UserID": true, "TenantID": true, "OwnerID": true, "PrincipalID": true,
		"SessionID": true, "ExecutionID": true, "Bucket": true, "Key": true,
		"Endpoint": true, "Credential": true, "TTL": true,
	}
	for _, request := range requests {
		typ := reflect.TypeOf(request)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if forbidden[field.Name] {
				t.Errorf("%s exposes forbidden authority/storage field %s", typ.Name(), field.Name)
			}
		}
	}
	// ExtensionID is provenance metadata inside Source, never caller-selected ownership.
	if _, ok := reflect.TypeOf(PutResultRequest{}).FieldByName("Source"); !ok {
		t.Fatal("put request lost exact producer source identity")
	}
}

func TestManifestOptionalCapabilitiesAreForwardCompatible(t *testing.T) {
	m := mustParse(t, validManifestYAML)
	m.OptionalCapabilities = []Capability{CapEmbeddingInvoke, "future.optional"}
	if err := m.Validate(); err != nil {
		t.Fatalf("unknown optional must be accepted: %v", err)
	}
	if !m.HasCapability(CapEmbeddingInvoke) || m.HasRequiredCapability(CapEmbeddingInvoke) {
		t.Fatal("optional semantics lost")
	}
	requested := m.RequestedCapabilities()
	if requested[len(requested)-2] != CapEmbeddingInvoke || requested[len(requested)-1] != "future.optional" {
		t.Fatalf("requested union: %v", requested)
	}
	m.Capabilities = append(m.Capabilities, "future.required")
	if err := m.Validate(); err == nil {
		t.Fatal("unknown required capability accepted")
	}
}
