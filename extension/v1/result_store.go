package extensionv1

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxResultBytes       = 8 << 20
	MaxResultReadBytes   = 1 << 20
	MaxResultPageItems   = 100
	MaxResultPageFields  = 32
	MaxResultFieldBytes  = 256
	MaxResultCursorBytes = 512
	MaxResultRefBytes    = 512
)

type ResultReference struct {
	Ref string `json:"ref"`
}

// ResultSourceIdentity identifies the exact producing tool contract. Scope,
// bucket/key, endpoint, credentials and TTL are intentionally absent.
type ResultSourceIdentity struct {
	ExtensionID string `json:"extensionId"`
	ServerID    string `json:"serverId"`
	ToolName    string `json:"toolName"`
	ToolVersion string `json:"toolVersion,omitempty"`
	SchemaHash  string `json:"schemaHash"`
}

type PutResultRequest struct {
	Data     []byte               `json:"data"`
	Length   int64                `json:"length"`
	SHA256   string               `json:"sha256"`
	MIMEType string               `json:"mimeType"`
	Source   ResultSourceIdentity `json:"source"`
}

type ResultStatus string

const (
	ResultStatusAvailable   ResultStatus = "available"
	ResultStatusExpired     ResultStatus = "expired"
	ResultStatusUnavailable ResultStatus = "unavailable"
)

type ResultInfo struct {
	Reference  ResultReference      `json:"reference"`
	Status     ResultStatus         `json:"status"`
	Length     int64                `json:"length"`
	SHA256     string               `json:"sha256"`
	MIMEType   string               `json:"mimeType"`
	Source     ResultSourceIdentity `json:"source"`
	CreatedAt  time.Time            `json:"createdAt"`
	ObservedAt time.Time            `json:"observedAt"`
	ExpiresAt  time.Time            `json:"expiresAt,omitempty"`
}

type ResultReceipt struct {
	Info ResultInfo `json:"info"`
}

type ReadResultRequest struct {
	Reference ResultReference `json:"reference"`
	Offset    int64           `json:"offset"`
	Limit     int             `json:"limit"`
}

type ReadResultResponse struct {
	Info     ResultInfo `json:"info"`
	Offset   int64      `json:"offset"`
	Data     []byte     `json:"data"`
	Complete bool       `json:"complete"`
}

type ReadResultPageRequest struct {
	Reference ResultReference `json:"reference"`
	Cursor    string          `json:"cursor,omitempty"`
	Limit     int             `json:"limit"`
	Fields    []string        `json:"fields,omitempty"`
}

type ResultPage struct {
	Info       ResultInfo        `json:"info"`
	Items      []json.RawMessage `json:"items"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

type GetResultInfoRequest struct {
	Reference ResultReference `json:"reference"`
}

type ResultStoreService interface {
	PutResult(context.Context, PutResultRequest) (ResultReceipt, error)
	ReadResult(context.Context, ReadResultRequest) (ReadResultResponse, error)
	ReadResultPage(context.Context, ReadResultPageRequest) (ResultPage, error)
	GetResultInfo(context.Context, GetResultInfoRequest) (ResultInfo, error)
}

func (h HostContext) RequireResultWrite() (ResultStoreService, error) {
	return RequireService[ResultStoreService](h, CapResultWrite)
}

func (h HostContext) RequireResultRead() (ResultStoreService, error) {
	return RequireService[ResultStoreService](h, CapResultRead)
}

func (r ResultReference) Validate() error {
	if len(r.Ref) == 0 || len(r.Ref) > MaxResultRefBytes || !utf8.ValidString(r.Ref) || strings.TrimSpace(r.Ref) != r.Ref {
		return NewError(CodeInvalidArgument, "result reference is empty, malformed, or too long")
	}
	for _, value := range r.Ref {
		if value < 0x21 || value == 0x7f {
			return NewError(CodeInvalidArgument, "result reference contains a control character")
		}
	}
	return nil
}

func (s ResultSourceIdentity) Validate() error {
	for name, value := range map[string]string{"extensionId": s.ExtensionID, "serverId": s.ServerID, "toolName": s.ToolName} {
		if strings.TrimSpace(value) == "" || len(value) > 256 || !utf8.ValidString(value) {
			return Errorf(CodeInvalidArgument, "result source %s is invalid", name)
		}
	}
	if len(s.ToolVersion) > 256 || !utf8.ValidString(s.ToolVersion) {
		return NewError(CodeInvalidArgument, "result source toolVersion is invalid")
	}
	if err := validateSHA256(s.SchemaHash, "result source schemaHash"); err != nil {
		return err
	}
	return nil
}

func (r PutResultRequest) Validate() error {
	if len(r.Data) == 0 || len(r.Data) > MaxResultBytes {
		return Errorf(CodeInvalidArgument, "result data must be between 1 and %d bytes", MaxResultBytes)
	}
	if r.Length != int64(len(r.Data)) {
		return NewError(CodeInvalidArgument, "result length does not match data")
	}
	if err := validateSHA256(r.SHA256, "result sha256"); err != nil {
		return err
	}
	sum := sha256.Sum256(r.Data)
	if hex.EncodeToString(sum[:]) != r.SHA256 {
		return NewError(CodeInvalidArgument, "result sha256 does not match data")
	}
	if err := validateMIME(r.MIMEType); err != nil {
		return err
	}
	return r.Source.Validate()
}

func (r ReadResultRequest) Validate() error {
	if err := r.Reference.Validate(); err != nil {
		return err
	}
	if r.Offset < 0 || r.Limit <= 0 || r.Limit > MaxResultReadBytes {
		return Errorf(CodeInvalidArgument, "result read offset/limit is invalid (maximum %d bytes)", MaxResultReadBytes)
	}
	return nil
}

func (r ReadResultPageRequest) Validate() error {
	if err := r.Reference.Validate(); err != nil {
		return err
	}
	if r.Limit <= 0 || r.Limit > MaxResultPageItems || len(r.Cursor) > MaxResultCursorBytes || !utf8.ValidString(r.Cursor) {
		return NewError(CodeInvalidArgument, "result page cursor or limit is invalid")
	}
	if len(r.Fields) > MaxResultPageFields {
		return Errorf(CodeInvalidArgument, "result page fields exceed %d", MaxResultPageFields)
	}
	seen := map[string]bool{}
	for _, field := range r.Fields {
		if len(field) > MaxResultFieldBytes || !validJSONPointer(field) || seen[field] {
			return NewError(CodeInvalidArgument, "result page field must be a unique bounded JSON pointer")
		}
		seen[field] = true
	}
	return nil
}

func (r GetResultInfoRequest) Validate() error { return r.Reference.Validate() }

func (i ResultInfo) Validate() error {
	if err := i.Reference.Validate(); err != nil {
		return err
	}
	if i.Status != ResultStatusAvailable && i.Status != ResultStatusExpired && i.Status != ResultStatusUnavailable {
		return NewError(CodeIncompatible, "result info has an unknown status")
	}
	if i.Length <= 0 || i.Length > MaxResultBytes {
		return NewError(CodeIncompatible, "result info length is outside the supported bounds")
	}
	if err := validateSHA256(i.SHA256, "result info sha256"); err != nil {
		return NewError(CodeIncompatible, err.Error())
	}
	if err := validateMIME(i.MIMEType); err != nil {
		return NewError(CodeIncompatible, err.Error())
	}
	if err := i.Source.Validate(); err != nil {
		return NewError(CodeIncompatible, err.Error())
	}
	if i.CreatedAt.IsZero() || i.ObservedAt.IsZero() || i.ObservedAt.Before(i.CreatedAt) || (!i.ExpiresAt.IsZero() && !i.ExpiresAt.After(i.CreatedAt)) {
		return NewError(CodeIncompatible, "result info timestamps are inconsistent")
	}
	return nil
}

func validateSHA256(value, field string) error {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return Errorf(CodeInvalidArgument, "%s must be a lowercase SHA-256 hex digest", field)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return Errorf(CodeInvalidArgument, "%s must be a lowercase SHA-256 hex digest", field)
	}
	return nil
}

func validJSONPointer(value string) bool {
	if value == "" || value[0] != '/' || !utf8.ValidString(value) {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] != '~' {
			continue
		}
		if i+1 >= len(value) || (value[i+1] != '0' && value[i+1] != '1') {
			return false
		}
		i++
	}
	return true
}

func validateMIME(value string) error {
	if len(value) == 0 || len(value) > 255 || strings.ContainsAny(value, "\r\n") {
		return NewError(CodeInvalidArgument, "result MIME type is invalid")
	}
	if _, _, err := mime.ParseMediaType(value); err != nil {
		return NewError(CodeInvalidArgument, "result MIME type is invalid")
	}
	return nil
}
