package server

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/couchbase/gocb/v2"
	"github.com/couchbase/stellar-nebula/protos"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/runtime/protoiface"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func casToPs(cas gocb.Cas) *protos.Cas {
	return &protos.Cas{
		Value: uint64(cas),
	}
}

func casFromPs(cas *protos.Cas) gocb.Cas {
	return gocb.Cas(cas.Value)
}

func timeToPs(when time.Time) *timestamppb.Timestamp {
	if when.IsZero() {
		return nil
	}
	return timestamppb.New(when)
}

func timeFromPs(ts *timestamppb.Timestamp) time.Time {
	return ts.AsTime()
}

func durationToPs(dura time.Duration) *durationpb.Duration {
	return durationpb.New(dura)
}

func durationFromPs(d *durationpb.Duration) time.Duration {
	return d.AsDuration()
}

func tokenToPs(token *gocb.MutationToken) *protos.MutationToken {
	if token == nil {
		return nil
	}

	return &protos.MutationToken{
		BucketName:  token.BucketName(),
		VbucketId:   uint32(token.PartitionID()),
		VbucketUuid: token.PartitionUUID(),
		SeqNo:       token.SequenceNumber(),
	}
}

func durabilityLevelFromPs(dl *protos.DurabilityLevel) (gocb.DurabilityLevel, *status.Status) {
	if dl == nil {
		return gocb.DurabilityLevelNone, nil
	}

	switch *dl {
	case protos.DurabilityLevel_MAJORITY:
		return gocb.DurabilityLevelMajority, nil
	case protos.DurabilityLevel_MAJORITY_AND_PERSIST_TO_ACTIVE:
		return gocb.DurabilityLevelMajorityAndPersistOnMaster, nil
	case protos.DurabilityLevel_PERSIST_TO_MAJORITY:
		return gocb.DurabilityLevelPersistToMajority, nil
	}

	return gocb.DurabilityLevelNone, status.New(codes.InvalidArgument, "invalid durability level options specified")
}

func cbErrToPsStatus(err error) *status.Status {
	log.Printf("handling error: %+v", err)

	var errorDetails protoiface.MessageV1

	var keyValueContext *gocb.KeyValueError
	if errors.As(err, &keyValueContext) {
		// TODO(bret19): Need to include more error context here
		errorDetails = &protos.ErrorInfo{
			Reason: keyValueContext.ErrorName,
			Metadata: map[string]string{
				"bucket":     keyValueContext.BucketName,
				"scope":      keyValueContext.ScopeName,
				"collection": keyValueContext.CollectionName,
				"key":        keyValueContext.DocumentID,
			},
		}
	}

	makeError := func(c codes.Code, msg string) *status.Status {
		st := status.New(c, msg)
		if errorDetails != nil {
			st, _ = st.WithDetails(errorDetails)
		}
		return st
	}

	// this never actually makes it back to the GRPC client, but we
	// do the translation here anyways...
	if errors.Is(err, context.Canceled) {
		return makeError(codes.Canceled, "request canceled")
	}

	// TODO(brett19): Need to provide translation for more errors
	if errors.Is(err, gocb.ErrDocumentNotFound) {
		return makeError(codes.NotFound, "document not found")
	} else if errors.Is(err, gocb.ErrDocumentExists) {
		return makeError(codes.AlreadyExists, "document already exists")
	}

	return makeError(codes.Internal, "an unknown error occurred")
}

func cbErrToPs(err error) error {
	return cbErrToPsStatus(err).Err()
}
