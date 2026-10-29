package testkit

import (
	"context"
	"sync"

	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
)

type FakeEmbedding struct {
	mu        sync.Mutex
	Requests  []extv1.EmbeddingRequest
	Response  extv1.EmbeddingResponse
	Err       error
	EmbedFunc func(context.Context, extv1.EmbeddingRequest) (extv1.EmbeddingResponse, error)
}

func (f *FakeEmbedding) Embed(ctx context.Context, request extv1.EmbeddingRequest) (extv1.EmbeddingResponse, error) {
	if err := ctx.Err(); err != nil {
		return extv1.EmbeddingResponse{}, err
	}
	f.mu.Lock()
	f.Requests = append(f.Requests, request)
	fn, response, err := f.EmbedFunc, f.Response, f.Err
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, request)
	}
	return response, err
}

func (f *FakeEmbedding) History() []extv1.EmbeddingRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]extv1.EmbeddingRequest(nil), f.Requests...)
}

type FakeResultStore struct {
	mu           sync.Mutex
	PutRequests  []extv1.PutResultRequest
	ReadRequests []extv1.ReadResultRequest
	PageRequests []extv1.ReadResultPageRequest
	InfoRequests []extv1.GetResultInfoRequest
	PutFunc      func(context.Context, extv1.PutResultRequest) (extv1.ResultReceipt, error)
	ReadFunc     func(context.Context, extv1.ReadResultRequest) (extv1.ReadResultResponse, error)
	PageFunc     func(context.Context, extv1.ReadResultPageRequest) (extv1.ResultPage, error)
	InfoFunc     func(context.Context, extv1.GetResultInfoRequest) (extv1.ResultInfo, error)
}

func (f *FakeResultStore) PutResult(ctx context.Context, request extv1.PutResultRequest) (extv1.ResultReceipt, error) {
	f.mu.Lock()
	f.PutRequests = append(f.PutRequests, request)
	fn := f.PutFunc
	f.mu.Unlock()
	if fn == nil {
		return extv1.ResultReceipt{}, extv1.NewError(extv1.CodeNotFound, "fake result response not configured")
	}
	return fn(ctx, request)
}

func (f *FakeResultStore) ReadResult(ctx context.Context, request extv1.ReadResultRequest) (extv1.ReadResultResponse, error) {
	f.mu.Lock()
	f.ReadRequests = append(f.ReadRequests, request)
	fn := f.ReadFunc
	f.mu.Unlock()
	if fn == nil {
		return extv1.ReadResultResponse{}, extv1.NewError(extv1.CodeNotFound, "fake result response not configured")
	}
	return fn(ctx, request)
}

func (f *FakeResultStore) ReadResultPage(ctx context.Context, request extv1.ReadResultPageRequest) (extv1.ResultPage, error) {
	f.mu.Lock()
	f.PageRequests = append(f.PageRequests, request)
	fn := f.PageFunc
	f.mu.Unlock()
	if fn == nil {
		return extv1.ResultPage{}, extv1.NewError(extv1.CodeNotFound, "fake result response not configured")
	}
	return fn(ctx, request)
}

func (f *FakeResultStore) GetResultInfo(ctx context.Context, request extv1.GetResultInfoRequest) (extv1.ResultInfo, error) {
	f.mu.Lock()
	f.InfoRequests = append(f.InfoRequests, request)
	fn := f.InfoFunc
	f.mu.Unlock()
	if fn == nil {
		return extv1.ResultInfo{}, extv1.NewError(extv1.CodeNotFound, "fake result response not configured")
	}
	return fn(ctx, request)
}

type resultWriteOnly struct{ *FakeResultStore }

func (resultWriteOnly) ReadResult(context.Context, extv1.ReadResultRequest) (extv1.ReadResultResponse, error) {
	return extv1.ReadResultResponse{}, extv1.ErrCapabilityUnavailable
}
func (resultWriteOnly) ReadResultPage(context.Context, extv1.ReadResultPageRequest) (extv1.ResultPage, error) {
	return extv1.ResultPage{}, extv1.ErrCapabilityUnavailable
}
func (resultWriteOnly) GetResultInfo(context.Context, extv1.GetResultInfoRequest) (extv1.ResultInfo, error) {
	return extv1.ResultInfo{}, extv1.ErrCapabilityUnavailable
}

type resultReadOnly struct{ *FakeResultStore }

func (resultReadOnly) PutResult(context.Context, extv1.PutResultRequest) (extv1.ResultReceipt, error) {
	return extv1.ResultReceipt{}, extv1.ErrCapabilityUnavailable
}

func WithEmbedding(f *FakeEmbedding) Option {
	return func(h *Host) {
		if f != nil {
			h.Embedding = f
		}
	}
}

func WithResultWrite(f *FakeResultStore) Option {
	return func(h *Host) {
		if f != nil {
			h.Results = f
			h.resultWrite = true
		}
	}
}

func WithResultRead(f *FakeResultStore) Option {
	return func(h *Host) {
		if f != nil {
			h.Results = f
			h.resultRead = true
		}
	}
}
