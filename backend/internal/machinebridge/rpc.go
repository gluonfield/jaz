package machinebridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
)

type RPCError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type Frame struct {
	ID     string          `json:"id"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

func toRPCError(err error) *RPCError {
	if err == nil {
		return nil
	}
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) && rpcErr != nil {
		return rpcErr
	}
	var code ErrorCode
	switch {
	case errors.Is(err, ErrSymlinkEscape):
		code = CodeSymlinkEscape
	case errors.Is(err, ErrPathOutsideGrant):
		code = CodePathOutsideGrant
	case errors.Is(err, ErrAccessDenied):
		code = CodeAccessDenied
	case errors.Is(err, ErrConflict):
		code = CodeConflict
	case errors.Is(err, ErrUnsupportedCapability):
		code = CodeUnsupportedCapability
	case errors.Is(err, ErrInvalidRequest):
		code = CodeInvalidRequest
	case errors.Is(err, ErrDeviceOffline):
		code = CodeDeviceOffline
	case errors.Is(err, ErrNotFound), errors.Is(err, fs.ErrNotExist):
		code = CodeNotFound
	default:
		code = CodeInternal
	}
	return &RPCError{Code: code, Message: err.Error()}
}
