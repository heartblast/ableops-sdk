package extserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"unicode/utf8"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

func semanticRequestScoped(ctx context.Context) error {
	_, hasIdentity := IdentityFrom(ctx)
	_, hasToken := requestTokenFrom(ctx)
	if !hasIdentity || !hasToken {
		return extv1.NewError(extv1.CodePermissionDenied, "Host service call requires a live Host-proxied request principal")
	}
	return nil
}

type embeddingService struct{ c *hostClient }

var _ extv1.EmbeddingService = embeddingService{}

func (s embeddingService) Embed(ctx context.Context, request extv1.EmbeddingRequest) (extv1.EmbeddingResponse, error) {
	if err := request.Validate(); err != nil {
		return extv1.EmbeddingResponse{}, err
	}
	if err := semanticRequestScoped(ctx); err != nil {
		return extv1.EmbeddingResponse{}, err
	}
	var response extv1.EmbeddingResponse
	if err := s.c.call(ctx, http.MethodPost, "/embedding/embed", request, &response); err != nil {
		return extv1.EmbeddingResponse{}, err
	}
	if err := response.ValidateFor(request); err != nil {
		return extv1.EmbeddingResponse{}, err
	}
	return response, nil
}

type resultStoreService struct {
	c        *hostClient
	canWrite bool
	canRead  bool
}

var _ extv1.ResultStoreService = (*resultStoreService)(nil)

func (s *resultStoreService) PutResult(ctx context.Context, request extv1.PutResultRequest) (extv1.ResultReceipt, error) {
	if !s.canWrite {
		return extv1.ResultReceipt{}, extv1.Errorf(extv1.CodeCapabilityUnavailable, "extension %s requires %s", s.c.extensionID, extv1.CapResultWrite)
	}
	if err := request.Validate(); err != nil {
		return extv1.ResultReceipt{}, err
	}
	if err := semanticRequestScoped(ctx); err != nil {
		return extv1.ResultReceipt{}, err
	}
	var response extv1.ResultReceipt
	if err := s.c.call(ctx, http.MethodPost, "/results/put", request, &response); err != nil {
		return extv1.ResultReceipt{}, err
	}
	if err := validateResultInfo(response.Info); err != nil {
		return extv1.ResultReceipt{}, err
	}
	if response.Info.Length != request.Length || response.Info.SHA256 != request.SHA256 || response.Info.MIMEType != request.MIMEType || response.Info.Source != request.Source {
		return extv1.ResultReceipt{}, extv1.NewError(extv1.CodeIncompatible, "result receipt metadata does not exactly match the put request")
	}
	return response, nil
}

func (s *resultStoreService) ReadResult(ctx context.Context, request extv1.ReadResultRequest) (extv1.ReadResultResponse, error) {
	if !s.canRead {
		return extv1.ReadResultResponse{}, extv1.Errorf(extv1.CodeCapabilityUnavailable, "extension %s requires %s", s.c.extensionID, extv1.CapResultRead)
	}
	if err := request.Validate(); err != nil {
		return extv1.ReadResultResponse{}, err
	}
	if err := semanticRequestScoped(ctx); err != nil {
		return extv1.ReadResultResponse{}, err
	}
	var response extv1.ReadResultResponse
	if err := s.c.call(ctx, http.MethodPost, "/results/read", request, &response); err != nil {
		return extv1.ReadResultResponse{}, err
	}
	if err := validateResultInfo(response.Info); err != nil {
		return extv1.ReadResultResponse{}, err
	}
	if response.Info.Reference != request.Reference || response.Offset != request.Offset || len(response.Data) > request.Limit || response.Offset+int64(len(response.Data)) > response.Info.Length {
		return extv1.ReadResultResponse{}, extv1.NewError(extv1.CodeIncompatible, "result read response exceeds or contradicts the requested bounds")
	}
	end := response.Offset + int64(len(response.Data))
	if response.Complete != (end == response.Info.Length) {
		return extv1.ReadResultResponse{}, extv1.NewError(extv1.CodeIncompatible, "result read completion marker is inconsistent")
	}
	if response.Offset == 0 && response.Complete {
		sum := sha256.Sum256(response.Data)
		if hex.EncodeToString(sum[:]) != response.Info.SHA256 {
			return extv1.ReadResultResponse{}, extv1.NewError(extv1.CodeIncompatible, "result read data does not match its SHA-256")
		}
	}
	return response, nil
}

func (s *resultStoreService) ReadResultPage(ctx context.Context, request extv1.ReadResultPageRequest) (extv1.ResultPage, error) {
	if !s.canRead {
		return extv1.ResultPage{}, extv1.Errorf(extv1.CodeCapabilityUnavailable, "extension %s requires %s", s.c.extensionID, extv1.CapResultRead)
	}
	if err := request.Validate(); err != nil {
		return extv1.ResultPage{}, err
	}
	if err := semanticRequestScoped(ctx); err != nil {
		return extv1.ResultPage{}, err
	}
	var response extv1.ResultPage
	if err := s.c.call(ctx, http.MethodPost, "/results/page", request, &response); err != nil {
		return extv1.ResultPage{}, err
	}
	if err := validateResultInfo(response.Info); err != nil {
		return extv1.ResultPage{}, err
	}
	if response.Info.Reference != request.Reference || len(response.Items) > request.Limit || len(response.NextCursor) > extv1.MaxResultCursorBytes || !utf8.ValidString(response.NextCursor) {
		return extv1.ResultPage{}, extv1.NewError(extv1.CodeIncompatible, "result page response exceeds or contradicts the requested bounds")
	}
	total := 0
	for _, item := range response.Items {
		if !json.Valid(item) {
			return extv1.ResultPage{}, extv1.NewError(extv1.CodeIncompatible, "result page contains invalid JSON")
		}
		total += len(item)
	}
	if total > extv1.MaxResultReadBytes {
		return extv1.ResultPage{}, extv1.NewError(extv1.CodeIncompatible, "result page exceeds the byte bound")
	}
	return response, nil
}

func (s *resultStoreService) GetResultInfo(ctx context.Context, request extv1.GetResultInfoRequest) (extv1.ResultInfo, error) {
	if !s.canRead {
		return extv1.ResultInfo{}, extv1.Errorf(extv1.CodeCapabilityUnavailable, "extension %s requires %s", s.c.extensionID, extv1.CapResultRead)
	}
	if err := request.Validate(); err != nil {
		return extv1.ResultInfo{}, err
	}
	if err := semanticRequestScoped(ctx); err != nil {
		return extv1.ResultInfo{}, err
	}
	var response extv1.ResultInfo
	if err := s.c.call(ctx, http.MethodPost, "/results/info", request, &response); err != nil {
		return extv1.ResultInfo{}, err
	}
	if err := validateResultInfo(response); err != nil {
		return extv1.ResultInfo{}, err
	}
	if response.Reference != request.Reference {
		return extv1.ResultInfo{}, extv1.NewError(extv1.CodeIncompatible, "result info reference does not match the request")
	}
	return response, nil
}

func validateResultInfo(info extv1.ResultInfo) error {
	if err := info.Validate(); err != nil {
		if extv1.CodeOf(err) == extv1.CodeInvalidArgument {
			return extv1.NewError(extv1.CodeIncompatible, err.Error())
		}
		return err
	}
	return nil
}
