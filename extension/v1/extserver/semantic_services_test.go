package extserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

func semanticModel() extv1.EmbeddingModelIdentity {
	return extv1.EmbeddingModelIdentity{ModelID: "e5", ModelRevision: "r1", TokenizerRevision: "t1", Dimensions: 2, PoolingNormalization: "l2", PreprocessingVersion: "query-v1"}
}

func semanticInfo(data []byte) extv1.ResultInfo {
	sum := sha256.Sum256(data)
	created := time.Unix(100, 0).UTC()
	return extv1.ResultInfo{
		Reference: extv1.ResultReference{Ref: "opaque-result-ref"}, Status: extv1.ResultStatusAvailable,
		Length: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), MIMEType: "application/json",
		Source:    extv1.ResultSourceIdentity{ExtensionID: "ai-ext", ServerID: "kafka", ToolName: "lag", SchemaHash: strings.Repeat("a", 64)},
		CreatedAt: created, ObservedAt: created.Add(time.Second), ExpiresAt: created.Add(time.Hour),
	}
}

func TestSemanticServicesUseFixedHostEndpointsAndExactMetadata(t *testing.T) {
	data := []byte(`{"ok":true}`)
	info := semanticInfo(data)
	var paths []string
	host, _ := hostFor(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get(HeaderOnBehalfOf) != "alice" || r.Header.Get(HeaderRequestToken) != "lease-1" {
			t.Errorf("missing request scope headers on %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/v1/embedding/embed":
			var request extv1.EmbeddingRequest
			_ = json.NewDecoder(r.Body).Decode(&request)
			_ = json.NewEncoder(w).Encode(extv1.EmbeddingResponse{Model: request.Model, Vectors: []extv1.EmbeddingVector{{Index: 0, Values: []float32{0.6, 0.8}}}})
		case "/v1/results/put":
			_ = json.NewEncoder(w).Encode(extv1.ResultReceipt{Info: info})
		case "/v1/results/read":
			_ = json.NewEncoder(w).Encode(extv1.ReadResultResponse{Info: info, Data: data, Complete: true})
		case "/v1/results/page":
			_ = json.NewEncoder(w).Encode(extv1.ResultPage{Info: info, Items: []json.RawMessage{json.RawMessage(data)}})
		case "/v1/results/info":
			_ = json.NewEncoder(w).Encode(info)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}, 0, extv1.CapEmbeddingInvoke, extv1.CapResultWrite, extv1.CapResultRead)
	ctx := requestCtx(context.Background())
	embed, err := host.RequireEmbedding()
	if err != nil {
		t.Fatal(err)
	}
	request := extv1.EmbeddingRequest{Inputs: []extv1.EmbeddingInput{{Index: 0, Text: "query: lag"}}, Model: semanticModel()}
	if _, err := embed.Embed(ctx, request); err != nil {
		t.Fatal(err)
	}
	store, err := host.RequireResultWrite()
	if err != nil {
		t.Fatal(err)
	}
	put := extv1.PutResultRequest{Data: data, Length: info.Length, SHA256: info.SHA256, MIMEType: info.MIMEType, Source: info.Source}
	if _, err := store.PutResult(ctx, put); err != nil {
		t.Fatal(err)
	}
	read, _ := host.RequireResultRead()
	ref := info.Reference
	if _, err := read.ReadResult(ctx, extv1.ReadResultRequest{Reference: ref, Limit: len(data)}); err != nil {
		t.Fatal(err)
	}
	if _, err := read.ReadResultPage(ctx, extv1.ReadResultPageRequest{Reference: ref, Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := read.GetResultInfo(ctx, extv1.GetResultInfoRequest{Reference: ref}); err != nil {
		t.Fatal(err)
	}
	want := []string{"/v1/embedding/embed", "/v1/results/put", "/v1/results/read", "/v1/results/page", "/v1/results/info"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("paths=%v", paths)
	}
}

func TestSemanticServicesFailClosedBeforeNetwork(t *testing.T) {
	var hits atomic.Int32
	host, _ := hostFor(t, func(http.ResponseWriter, *http.Request) { hits.Add(1) }, 0, extv1.CapEmbeddingInvoke, extv1.CapResultWrite)
	embed, _ := host.RequireEmbedding()
	request := extv1.EmbeddingRequest{Inputs: []extv1.EmbeddingInput{{Index: 0, Text: "x"}}, Model: semanticModel()}
	if _, err := embed.Embed(context.Background(), request); extv1.CodeOf(err) != extv1.CodePermissionDenied {
		t.Fatalf("background embedding: %v", err)
	}
	request.Inputs[0].Index = 1
	if _, err := embed.Embed(requestCtx(context.Background()), request); extv1.CodeOf(err) != extv1.CodeInvalidArgument {
		t.Fatalf("bad embedding: %v", err)
	}
	write, _ := host.RequireResultWrite()
	if _, err := write.ReadResult(requestCtx(context.Background()), extv1.ReadResultRequest{}); extv1.CodeOf(err) != extv1.CodeCapabilityUnavailable {
		t.Fatalf("write grant read: %v", err)
	}
	if _, err := host.RequireResultRead(); !errors.Is(err, extv1.ErrCapabilityUnavailable) {
		t.Fatalf("read resolver: %v", err)
	}
	if _, err := write.PutResult(requestCtx(context.Background()), extv1.PutResultRequest{Data: []byte("x"), Length: 2}); extv1.CodeOf(err) != extv1.CodeInvalidArgument {
		t.Fatalf("bad put: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("host contacted %d times", hits.Load())
	}
}

func TestSemanticServiceMapsHostErrors(t *testing.T) {
	host, _ := hostFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(extv1.NewError(extv1.CodeNotFound, "result not found"))
	}, 0, extv1.CapResultRead)
	store, _ := host.RequireResultRead()
	_, err := store.GetResultInfo(requestCtx(context.Background()), extv1.GetResultInfoRequest{Reference: extv1.ResultReference{Ref: "opaque-missing-ref"}})
	if HostErrorCode(err) != extv1.CodeNotFound || !errors.Is(err, extv1.NewError(extv1.CodeNotFound, "")) {
		t.Fatalf("mapping: %v", err)
	}
}
