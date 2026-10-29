package extensionv1

import (
	"context"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	MaxEmbeddingInputs          = 64
	MaxEmbeddingInputBytes      = 32 << 10
	MaxEmbeddingTotalInputBytes = 256 << 10
	MaxEmbeddingDimensions      = 4096
)

// EmbeddingModelIdentity pins every property that can change vector meaning.
// It deliberately contains no endpoint or credential.
type EmbeddingModelIdentity struct {
	ModelID              string `json:"modelId"`
	ModelRevision        string `json:"modelRevision"`
	TokenizerRevision    string `json:"tokenizerRevision"`
	Dimensions           int    `json:"dimensions"`
	PoolingNormalization string `json:"poolingNormalization"`
	PreprocessingVersion string `json:"preprocessingVersion"`
}

type EmbeddingInput struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

type EmbeddingRequest struct {
	Inputs []EmbeddingInput       `json:"inputs"`
	Model  EmbeddingModelIdentity `json:"model"`
}

type EmbeddingVector struct {
	Index  int       `json:"index"`
	Values []float32 `json:"values"`
}

type EmbeddingResponse struct {
	Model   EmbeddingModelIdentity `json:"model"`
	Vectors []EmbeddingVector      `json:"vectors"`
}

// EmbeddingService invokes the Host-managed embedding provider. Request scope
// is required by process clients; callers never select an endpoint or credential.
type EmbeddingService interface {
	Embed(context.Context, EmbeddingRequest) (EmbeddingResponse, error)
}

func (h HostContext) RequireEmbedding() (EmbeddingService, error) {
	return RequireService[EmbeddingService](h, CapEmbeddingInvoke)
}

func (m EmbeddingModelIdentity) Validate() error {
	for name, value := range map[string]string{
		"modelId": m.ModelID, "modelRevision": m.ModelRevision,
		"tokenizerRevision": m.TokenizerRevision, "preprocessingVersion": m.PreprocessingVersion,
	} {
		if strings.TrimSpace(value) == "" || len(value) > 256 || !utf8.ValidString(value) {
			return Errorf(CodeInvalidArgument, "embedding %s must be non-empty valid UTF-8 and at most 256 bytes", name)
		}
	}
	if m.Dimensions <= 0 || m.Dimensions > MaxEmbeddingDimensions {
		return Errorf(CodeInvalidArgument, "embedding dimensions must be between 1 and %d", MaxEmbeddingDimensions)
	}
	if m.PoolingNormalization != "l2" && m.PoolingNormalization != "none" {
		return NewError(CodeInvalidArgument, "embedding poolingNormalization must be l2 or none")
	}
	return nil
}

func (r EmbeddingRequest) Validate() error {
	if err := r.Model.Validate(); err != nil {
		return err
	}
	if len(r.Inputs) == 0 || len(r.Inputs) > MaxEmbeddingInputs {
		return Errorf(CodeInvalidArgument, "embedding input count must be between 1 and %d", MaxEmbeddingInputs)
	}
	total := 0
	for i, input := range r.Inputs {
		if input.Index != i {
			return Errorf(CodeInvalidArgument, "embedding input index %d is not the expected %d", input.Index, i)
		}
		if strings.TrimSpace(input.Text) == "" || !utf8.ValidString(input.Text) {
			return Errorf(CodeInvalidArgument, "embedding input %d must be non-empty valid UTF-8", i)
		}
		if len(input.Text) > MaxEmbeddingInputBytes {
			return Errorf(CodeInvalidArgument, "embedding input %d exceeds %d bytes", i, MaxEmbeddingInputBytes)
		}
		total += len(input.Text)
	}
	if total > MaxEmbeddingTotalInputBytes {
		return Errorf(CodeInvalidArgument, "embedding inputs exceed %d total bytes", MaxEmbeddingTotalInputBytes)
	}
	return nil
}

func (r EmbeddingResponse) ValidateFor(request EmbeddingRequest) error {
	if r.Model != request.Model {
		return NewError(CodeIncompatible, "embedding response model identity does not match the request")
	}
	if len(r.Vectors) != len(request.Inputs) {
		return NewError(CodeIncompatible, "embedding response vector count does not match the request")
	}
	for i, vector := range r.Vectors {
		if vector.Index != request.Inputs[i].Index {
			return Errorf(CodeIncompatible, "embedding response index %d is out of order", vector.Index)
		}
		if len(vector.Values) != request.Model.Dimensions {
			return Errorf(CodeIncompatible, "embedding response vector %d has dimension %d, expected %d", i, len(vector.Values), request.Model.Dimensions)
		}
		norm2 := float64(0)
		for _, value := range vector.Values {
			v := float64(value)
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return Errorf(CodeIncompatible, "embedding response vector %d contains a non-finite value", i)
			}
			norm2 += v * v
		}
		if norm2 == 0 {
			return Errorf(CodeIncompatible, "embedding response vector %d is zero", i)
		}
		if request.Model.PoolingNormalization == "l2" && math.Abs(math.Sqrt(norm2)-1) > 1e-3 {
			return Errorf(CodeIncompatible, "embedding response vector %d is not L2 normalized", i)
		}
	}
	return nil
}
