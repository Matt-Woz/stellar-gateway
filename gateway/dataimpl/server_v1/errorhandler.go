package server_v1

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/couchbase/gocbcorex/cbqueryx"
	"github.com/couchbase/gocbcorex/cbsearchx"
	"github.com/couchbase/gocbcorex/memdx"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/runtime/protoiface"

	epb "google.golang.org/genproto/googleapis/rpc/errdetails"
)

/*
INVALID_ARGUMENT - The client sent a value which could never be considered correct.
FAILED_PRECONDITION - Something is in a state making the request invalid, retrying _COULD_ help.
OUT_OF_RANGE - More specific version of FAILED_PRECONDITION, useful to allow clients to iterate results until hitting this.
UNAUTHENTICATED - Occurs when credentials are not sent for something that must be authenticated.
PERMISSION_DENIED - Credentials were sent, but are insufficient.
NOT_FOUND - More specific version of FAILED_PRECONDITION where the document must exist, but does not.
ABORTED - Operation was running but was unambiguously aborted, must be retried at a higher level than at the request level.
ALREADY_EXISTS - More specific version of FAILED_PRECONDITION where the document must not exist, but does.
RESOURCE_EXHAUSTED - Indicates that some transient resource was exhausted, this could be quotas, limits, etc...  Implies retriability.
CANCELLED - Happens when a client explicitly cancels an operation.
DATA_LOSS - Indicates that data was lost unexpectedly and the operation cannot succeed.
UNKNOWN - Specified when no information about the cause of the error is available, otherwise INTERNAL should be used.
INTERNAL - Indicates any sort of error where the protocol wont provide parseable details to the client.
NOT_IMPLEMENTED - Indicates that a feature is not implemented, either a whole RPC, or possibly an option.
UNAVAILABLE - Cannot access the resource for some reason, clients can generate this if the server isn't available.
DEADLINE_EXCEEDED - Timeout occurred while processing.
*/

type ErrorHandler struct {
	Logger *zap.Logger
	Debug  bool
}

func (e ErrorHandler) tryAttachStatusDetails(st *status.Status, details ...protoiface.MessageV1) *status.Status {
	// try to attach the details
	if newSt, err := st.WithDetails(details...); err == nil {
		return newSt
	}

	// if we failed to attach the details, just return the original error
	return st
}

func (e ErrorHandler) tryAttachExtraContext(st *status.Status, baseErr error) *status.Status {
	var memdSrvErr *memdx.ServerErrorWithContext
	if errors.As(baseErr, &memdSrvErr) {
		parsedCtx := memdSrvErr.ParseContext()
		if parsedCtx.Ref != "" {
			st = e.tryAttachStatusDetails(st, &epb.RequestInfo{
				RequestId: parsedCtx.Ref,
			})
		}
	}

	if e.Debug {
		st = e.tryAttachStatusDetails(st, &epb.DebugInfo{
			Detail: baseErr.Error(),
		})
	}

	return st
}

func (e ErrorHandler) NewInternalStatus() *status.Status {
	st := status.New(codes.Internal, "An internal error occurred.")
	return st
}

func (e ErrorHandler) NewUnknownStatus(baseErr error) *status.Status {
	var memdErr *memdx.ServerError
	if errors.As(baseErr, &memdErr) {
		return status.New(codes.Unknown,
			fmt.Sprintf("An unknown memcached error occurred (status: %d).", memdErr.Status))
	}

	var queryErr *cbqueryx.QueryServerErrors
	if errors.As(baseErr, &queryErr) {
		var queryErrDescs []string
		for _, querySubErr := range queryErr.Errors {
			queryErrDescs = append(queryErrDescs, fmt.Sprintf("%d - %s", querySubErr.Code, querySubErr.Msg))
		}

		return status.New(codes.Unknown,
			fmt.Sprintf("An unknown query error occurred (descs: %s).", strings.Join(queryErrDescs, "; ")))
	}

	var searchErr *cbsearchx.ServerError
	if errors.As(baseErr, &searchErr) {
		return status.New(codes.Unknown,
			fmt.Sprintf("An unknown search error occurred (status: %d).", searchErr.StatusCode))
	}

	return status.New(codes.Unknown, "An unknown error occurred.")
}

func (e ErrorHandler) NewBucketMissingStatus(baseErr error, bucketName string) *status.Status {
	st := status.New(codes.NotFound,
		fmt.Sprintf("Bucket '%s' was not found.",
			bucketName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "bucket",
		ResourceName: bucketName,
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewBucketExistsStatus(baseErr error, bucketName string) *status.Status {
	st := status.New(codes.AlreadyExists,
		fmt.Sprintf("Bucket '%s' already existed.",
			bucketName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "bucket",
		ResourceName: bucketName,
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewScopeMissingStatus(baseErr error, bucketName, scopeName string) *status.Status {
	st := status.New(codes.NotFound,
		fmt.Sprintf("Scope '%s' not found in '%s'.",
			scopeName, bucketName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "scope",
		ResourceName: fmt.Sprintf("%s/%s", bucketName, scopeName),
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewCollectionMissingStatus(baseErr error, bucketName, scopeName, collectionName string) *status.Status {
	st := status.New(codes.NotFound,
		fmt.Sprintf("Collection '%s' not found in '%s/%s'.",
			collectionName, bucketName, scopeName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "collection",
		ResourceName: fmt.Sprintf("%s/%s/%s", bucketName, scopeName, collectionName),
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSearchIndexExistsStatus(baseErr error, indexName string) *status.Status {
	st := status.New(codes.NotFound,
		fmt.Sprintf("Search index '%s' not found.",
			indexName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "searchindex",
		ResourceName: indexName,
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSearchIndexMissingStatus(baseErr error, indexName string) *status.Status {
	st := status.New(codes.AlreadyExists,
		fmt.Sprintf("Search index '%s' already existed.",
			indexName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "searchindex",
		ResourceName: indexName,
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewDocMissingStatus(baseErr error, bucketName, scopeName, collectionName, docId string) *status.Status {
	st := status.New(codes.NotFound,
		fmt.Sprintf("Document '%s' not found in '%s/%s/%s'.",
			docId, bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "document",
		ResourceName: fmt.Sprintf("%s/%s/%s/%s", bucketName, scopeName, collectionName, docId),
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewDocExistsStatus(baseErr error, bucketName, scopeName, collectionName, docId string) *status.Status {
	st := status.New(codes.AlreadyExists,
		fmt.Sprintf("Document '%s' already existed in '%s/%s/%s'.",
			docId, bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "document",
		ResourceName: fmt.Sprintf("%s/%s/%s/%s", bucketName, scopeName, collectionName, docId),
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewDocCasMismatchStatus(baseErr error, bucketName, scopeName, collectionName, docId string) *status.Status {
	st := status.New(codes.FailedPrecondition,
		fmt.Sprintf("The specified CAS for '%s' in '%s/%s/%s' did not match.",
			docId, bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.PreconditionFailure{
		Violations: []*epb.PreconditionFailure_Violation{{
			Type:        "CAS",
			Subject:     fmt.Sprintf("%s/%s/%s/%s", bucketName, scopeName, collectionName, docId),
			Description: "",
		}},
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewDocLockedStatus(baseErr error, bucketName, scopeName, collectionName, docId string) *status.Status {
	st := status.New(codes.FailedPrecondition,
		fmt.Sprintf("Cannot perform a write operation against locked document '%s' in '%s/%s/%s'.",
			docId, bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.PreconditionFailure{
		Violations: []*epb.PreconditionFailure_Violation{{
			Type:        "LOCKED",
			Subject:     fmt.Sprintf("%s/%s/%s/%s", bucketName, scopeName, collectionName, docId),
			Description: "",
		}},
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewCollectionNoReadAccessStatus(baseErr error, bucketName, scopeName, collectionName string) *status.Status {
	st := status.New(codes.PermissionDenied,
		fmt.Sprintf("No permissions to read documents from '%s/%s/%s'.",
			bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "collection",
		ResourceName: fmt.Sprintf("%s/%s/%s", bucketName, scopeName, collectionName),
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewCollectionNoWriteAccessStatus(baseErr error, bucketName, scopeName, collectionName string) *status.Status {
	st := status.New(codes.PermissionDenied,
		fmt.Sprintf("No permissions to write documents into '%s/%s/%s'.",
			bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "collection",
		ResourceName: fmt.Sprintf("%s/%s/%s", bucketName, scopeName, collectionName),
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdDocTooDeepStatus(baseErr error, bucketName, scopeName, collectionName, docId string) *status.Status {
	st := status.New(codes.FailedPrecondition,
		fmt.Sprintf("Document '%s' JSON was too deep to parse in '%s/%s/%s'.",
			docId, bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.PreconditionFailure{
		Violations: []*epb.PreconditionFailure_Violation{{
			Type:        "DOC_TOO_DEEP",
			Subject:     fmt.Sprintf("%s/%s/%s/%s", bucketName, scopeName, collectionName, docId),
			Description: "",
		}},
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdDocNotJsonStatus(baseErr error, bucketName, scopeName, collectionName, docId string) *status.Status {
	st := status.New(codes.FailedPrecondition,
		fmt.Sprintf("Document '%s' was not JSON in '%s/%s/%s'.",
			docId, bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.PreconditionFailure{
		Violations: []*epb.PreconditionFailure_Violation{{
			Type:        "DOC_NOT_JSON",
			Subject:     fmt.Sprintf("%s/%s/%s/%s", bucketName, scopeName, collectionName, docId),
			Description: "",
		}},
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdPathNotFoundStatus(baseErr error, bucketName, scopeName, collectionName, docId, sdPath string) *status.Status {
	st := status.New(codes.NotFound,
		fmt.Sprintf("Subdocument path '%s' was not found in '%s' in '%s/%s/%s'.",
			sdPath, docId, bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "path",
		ResourceName: fmt.Sprintf("%s/%s/%s/%s/%s", bucketName, scopeName, collectionName, docId, sdPath),
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdPathExistsStatus(baseErr error, bucketName, scopeName, collectionName, docId, sdPath string) *status.Status {
	st := status.New(codes.AlreadyExists,
		fmt.Sprintf("Subdocument path '%s' already existed in '%s' in '%s/%s/%s'.",
			sdPath, docId, bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "path",
		ResourceName: fmt.Sprintf("%s/%s/%s/%s/%s", bucketName, scopeName, collectionName, docId, sdPath),
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdPathMismatchStatus(baseErr error, bucketName, scopeName, collectionName, docId, sdPath string) *status.Status {
	st := status.New(codes.FailedPrecondition,
		fmt.Sprintf("Document structure implied by path '%s' did not match document '%s' in '%s/%s/%s'.",
			sdPath, docId, bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.PreconditionFailure{
		Violations: []*epb.PreconditionFailure_Violation{{
			Type:        "PATH_MISMATCH",
			Subject:     sdPath,
			Description: "",
		}},
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdPathTooBigStatus(baseErr error, sdPath string) *status.Status {
	st := status.New(codes.InvalidArgument,
		fmt.Sprintf("Subdocument path '%s' is too long", sdPath))
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdBadValueStatus(baseErr error, sdPath string) *status.Status {
	st := status.New(codes.FailedPrecondition,
		fmt.Sprintf("Subdocument operation content for path '%s' would invalidate the JSON if added to the document.",
			sdPath))
	st = e.tryAttachStatusDetails(st, &epb.PreconditionFailure{
		Violations: []*epb.PreconditionFailure_Violation{{
			Type:        "WOULD_INVALIDATE_JSON",
			Subject:     sdPath,
			Description: "",
		}},
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdBadRangeStatus(baseErr error, bucketName, scopeName, collectionName, docId, sdPath string) *status.Status {
	st := status.New(codes.FailedPrecondition,
		fmt.Sprintf("The value at path '%s' is out of the valid range in document '%s' in '%s/%s/%s'.",
			sdPath, docId, bucketName, scopeName, collectionName))
	st = e.tryAttachStatusDetails(st, &epb.PreconditionFailure{
		Violations: []*epb.PreconditionFailure_Violation{{
			Type:        "PATH_VALUE_OUT_OF_RANGE",
			Subject:     sdPath,
			Description: "",
		}},
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdBadDeltaStatus(baseErr error, sdPath string) *status.Status {
	st := status.New(codes.InvalidArgument,
		fmt.Sprintf("Subdocument counter delta for path '%s' was invalid.",
			sdPath))
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdValueTooDeepStatus(baseErr error, sdPath string) *status.Status {
	st := status.New(codes.InvalidArgument,
		fmt.Sprintf("Subdocument operation content for path '%s' was too deep to parse.",
			sdPath))
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdXattrUnknownVattrStatus(baseErr error, sdPath string) *status.Status {
	st := status.New(codes.InvalidArgument,
		fmt.Sprintf("Subdocument path '%s' references an invalid virtual attribute.", sdPath))
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewSdPathInvalidStatus(baseErr error, sdPath string) *status.Status {
	st := status.New(codes.InvalidArgument,
		fmt.Sprintf("Invalid subdocument path syntax '%s'.", sdPath))
	// TODO(brett19): Probably should include invalid-argument error details.
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewUnsupportedFieldStatus(fieldPath string) *status.Status {
	st := status.New(codes.Unimplemented,
		fmt.Sprintf("The '%s' field is not currently supported", fieldPath))
	return st
}

func (e ErrorHandler) NewInvalidAuthHeaderStatus(baseErr error) *status.Status {
	st := status.New(codes.InvalidArgument, "Invalid authorization header format.")
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewNoAuthStatus() *status.Status {
	st := status.New(codes.Unauthenticated, "You must send authentication to use this endpoint.")
	return st
}

func (e ErrorHandler) NewInvalidCredentialsStatus() *status.Status {
	st := status.New(codes.PermissionDenied, "Your username or password is invalid.")
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "user",
		ResourceName: "",
		Description:  "",
	})
	return st
}

func (e ErrorHandler) NewInvalidQueryStatus(baseErr error, queryErrStr string) *status.Status {
	st := status.New(codes.InvalidArgument,
		fmt.Sprintf("Query parsing failed: %s", queryErrStr))
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewQueryNoAccessStatus(baseErr error) *status.Status {
	st := status.New(codes.PermissionDenied,
		"No permissions to query documents.")
	st = e.tryAttachStatusDetails(st, &epb.ResourceInfo{
		ResourceType: "user",
		ResourceName: "",
		Description:  "",
	})
	st = e.tryAttachExtraContext(st, baseErr)
	return st
}

func (e ErrorHandler) NewNeedIndexFieldsStatus() *status.Status {
	st := status.New(codes.InvalidArgument,
		"You must specify fields when creating a new index.")
	return st
}

func (e ErrorHandler) NewGenericStatus(err error) *status.Status {
	e.Logger.Error("handling generic error", zap.Error(err))

	if errors.Is(err, context.Canceled) {
		return status.New(codes.Canceled, "The request was cancelled.")
	} else if errors.Is(err, context.DeadlineExceeded) {
		return status.New(codes.DeadlineExceeded, "The request deadline was exceeded.")
	}

	return e.NewUnknownStatus(err)
}
