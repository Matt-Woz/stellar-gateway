package server_v1

import (
	"context"
	"errors"
	"time"

	"github.com/couchbase/gocbcorex/cbmgmtx"
	"github.com/couchbase/goprotostellar/genproto/admin_bucket_v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type BucketAdminServer struct {
	admin_bucket_v1.UnimplementedBucketAdminServiceServer

	logger       *zap.Logger
	errorHandler *ErrorHandler
	authHandler  *AuthHandler
}

func NewBucketAdminServer(
	logger *zap.Logger,
	errorHandler *ErrorHandler,
	authHandler *AuthHandler,
) *BucketAdminServer {
	return &BucketAdminServer{
		logger:       logger,
		errorHandler: errorHandler,
		authHandler:  authHandler,
	}
}

func (s *BucketAdminServer) ListBuckets(
	ctx context.Context,
	in *admin_bucket_v1.ListBucketsRequest,
) (*admin_bucket_v1.ListBucketsResponse, error) {
	agent, oboInfo, errSt := s.authHandler.GetHttpOboAgent(ctx, nil)
	if errSt != nil {
		return nil, errSt.Err()
	}

	result, err := agent.GetAllBuckets(ctx, &cbmgmtx.GetAllBucketsOptions{
		OnBehalfOf: oboInfo,
	})
	if err != nil {
		return nil, s.errorHandler.NewGenericStatus(err).Err()
	}

	var buckets []*admin_bucket_v1.ListBucketsResponse_Bucket
	for _, bucket := range result {
		bucketType, errSt := bucketTypeFromCbmgmtx(bucket.BucketType)
		if errSt != nil {
			return nil, errSt.Err()
		}

		evictionMode, errSt := evictionModeFromCbmgmtx(bucket.EvictionPolicy)
		if errSt != nil {
			return nil, errSt.Err()
		}

		compressionMode, errSt := compressionModeFromCbmgmtx(bucket.CompressionMode)
		if errSt != nil {
			return nil, errSt.Err()
		}

		minimumDurabilityLevel, errSt := durabilityLevelFromCbmgmtx(bucket.DurabilityMinLevel)
		if errSt != nil {
			return nil, errSt.Err()
		}

		storageBackend, errSt := storageBackendFromCbmgmtx(bucket.StorageBackend)
		if errSt != nil {
			return nil, errSt.Err()
		}

		conflictResolutionType, errSt := conflictResolutionTypeFromCbmgmtx(bucket.ConflictResolutionType)
		if errSt != nil {
			return nil, errSt.Err()
		}

		buckets = append(buckets, &admin_bucket_v1.ListBucketsResponse_Bucket{
			BucketName:             bucket.Name,
			FlushEnabled:           bucket.FlushEnabled,
			RamQuotaBytes:          bucket.RAMQuotaMB * 1024 * 1024,
			NumReplicas:            bucket.ReplicaNumber,
			ReplicaIndexes:         !bucket.ReplicaIndexDisabled,
			BucketType:             bucketType,
			EvictionMode:           evictionMode,
			MaxExpirySecs:          uint32(bucket.MaxTTL / time.Second),
			CompressionMode:        compressionMode,
			MinimumDurabilityLevel: minimumDurabilityLevel,
			StorageBackend:         storageBackend,
			ConflictResolutionType: conflictResolutionType,
		})
	}

	return &admin_bucket_v1.ListBucketsResponse{
		Buckets: buckets,
	}, nil
}

func (s *BucketAdminServer) CreateBucket(
	ctx context.Context,
	in *admin_bucket_v1.CreateBucketRequest,
) (*admin_bucket_v1.CreateBucketResponse, error) {
	agent, oboInfo, errSt := s.authHandler.GetHttpOboAgent(ctx, nil)
	if errSt != nil {
		return nil, errSt.Err()
	}

	flushEnabled := false
	if in.FlushEnabled != nil {
		flushEnabled = *in.FlushEnabled
	}

	replicaIndexes := false
	if in.ReplicaIndexes != nil {
		replicaIndexes = *in.ReplicaIndexes
	}

	bucketType, errSt := bucketTypeToCbmgmtx(in.BucketType)
	if errSt != nil {
		return nil, errSt.Err()
	}

	// TODO(brett19): Figure out how to properly handle default eviction type
	evictionPolicy := cbmgmtx.EvictionPolicyType("")
	if in.EvictionMode != nil {
		evictionPolicy, errSt = evictionModeToCbmgmtx(*in.EvictionMode)
		if errSt != nil {
			return nil, errSt.Err()
		}
	}

	maxExpiry := 0 * time.Second
	if in.MaxExpirySecs != nil {
		maxExpiry = (time.Duration)(*in.MaxExpirySecs) * time.Second
	}

	compressionMode := cbmgmtx.CompressionModePassive
	if in.CompressionMode != nil {
		compressionMode, errSt = compressionModeToCbmgmtx(*in.CompressionMode)
		if errSt != nil {
			return nil, errSt.Err()
		}
	}

	minimumDurabilityLevel := cbmgmtx.DurabilityLevelNone
	if in.MinimumDurabilityLevel != nil {
		minimumDurabilityLevel, errSt = durabilityLevelToCbmgmtx(*in.MinimumDurabilityLevel)
		if errSt != nil {
			return nil, errSt.Err()
		}
	}

	storageBackend := cbmgmtx.StorageBackendCouchstore
	if in.StorageBackend != nil {
		storageBackend, errSt = storageBackendToCbmgmtx(*in.StorageBackend)
		if errSt != nil {
			return nil, errSt.Err()
		}
	}

	conflictResolutionType := cbmgmtx.ConflictResolutionTypeSequenceNumber
	if in.ConflictResolutionType != nil {
		conflictResolutionType, errSt = conflictResolutionTypeToCbmgmtx(*in.ConflictResolutionType)
		if errSt != nil {
			return nil, errSt.Err()
		}
	}

	err := agent.CreateBucket(ctx, &cbmgmtx.CreateBucketOptions{
		OnBehalfOf: oboInfo,
		BucketName: in.BucketName,
		BucketSettings: cbmgmtx.BucketSettings{
			MutableBucketSettings: cbmgmtx.MutableBucketSettings{
				FlushEnabled:         flushEnabled,
				ReplicaIndexDisabled: replicaIndexes,
				RAMQuotaMB:           in.RamQuotaBytes / 1024 / 1024,
				ReplicaNumber:        in.NumReplicas,
				BucketType:           bucketType,
				EvictionPolicy:       evictionPolicy,
				MaxTTL:               maxExpiry,
				CompressionMode:      compressionMode,
				DurabilityMinLevel:   minimumDurabilityLevel,
				StorageBackend:       storageBackend,
			},
			ConflictResolutionType: conflictResolutionType,
		},
	})
	if err != nil {
		if errors.Is(err, cbmgmtx.ErrBucketExists) {
			return nil, s.errorHandler.NewBucketExistsStatus(err, in.BucketName).Err()
		}
		return nil, s.errorHandler.NewGenericStatus(err).Err()
	}

	return &admin_bucket_v1.CreateBucketResponse{}, nil
}

func (s *BucketAdminServer) UpdateBucket(
	ctx context.Context,
	in *admin_bucket_v1.UpdateBucketRequest,
) (*admin_bucket_v1.UpdateBucketResponse, error) {
	agent, oboInfo, errSt := s.authHandler.GetHttpOboAgent(ctx, nil)
	if errSt != nil {
		return nil, errSt.Err()
	}

	bucket, err := agent.GetBucket(ctx, &cbmgmtx.GetBucketOptions{
		BucketName: in.BucketName,
	})
	if err != nil {
		if errors.Is(err, cbmgmtx.ErrBucketNotFound) {
			return nil, s.errorHandler.NewBucketMissingStatus(err, in.BucketName).Err()
		}
		return nil, s.errorHandler.NewGenericStatus(err).Err()
	}

	newBucket := bucket.MutableBucketSettings

	if in.FlushEnabled != nil {
		newBucket.FlushEnabled = *in.FlushEnabled
	}

	if in.RamQuotaBytes != nil {
		newBucket.RAMQuotaMB = *in.RamQuotaBytes / 1024 / 1024
	}

	if in.NumReplicas != nil {
		newBucket.ReplicaNumber = *in.NumReplicas
	}

	if in.ReplicaIndexes != nil {
		newBucket.ReplicaIndexDisabled = !*in.ReplicaIndexes
	}

	if in.EvictionMode != nil {
		newBucket.EvictionPolicy, errSt = evictionModeToCbmgmtx(*in.EvictionMode)
		if errSt != nil {
			return nil, errSt.Err()
		}
	}

	if in.MaxExpirySecs != nil {
		newBucket.MaxTTL = time.Duration(*in.MaxExpirySecs) * time.Second
	}

	if in.CompressionMode != nil {
		newBucket.CompressionMode, errSt = compressionModeToCbmgmtx(*in.CompressionMode)
		if errSt != nil {
			return nil, errSt.Err()
		}
	}

	if in.MinimumDurabilityLevel != nil {
		newBucket.DurabilityMinLevel, errSt = durabilityLevelToCbmgmtx(*in.MinimumDurabilityLevel)
		if errSt != nil {
			return nil, errSt.Err()
		}
	}

	if in.ConflictResolutionType != nil {
		// TODO(brett19): Implement correct handling of conflict resolution type when gocb bug is fixed.
		return nil, status.Errorf(codes.Unimplemented, "conflict resolution type updates are not implemented")
	}

	err = agent.UpdateBucket(ctx, &cbmgmtx.UpdateBucketOptions{
		OnBehalfOf:            oboInfo,
		BucketName:            in.BucketName,
		MutableBucketSettings: newBucket,
	})
	if err != nil {
		if errors.Is(err, cbmgmtx.ErrBucketNotFound) {
			return nil, s.errorHandler.NewBucketMissingStatus(err, in.BucketName).Err()
		}
		return nil, s.errorHandler.NewGenericStatus(err).Err()
	}

	return &admin_bucket_v1.UpdateBucketResponse{}, nil
}

func (s *BucketAdminServer) DeleteBucket(
	ctx context.Context,
	in *admin_bucket_v1.DeleteBucketRequest,
) (*admin_bucket_v1.DeleteBucketResponse, error) {
	agent, oboInfo, errSt := s.authHandler.GetHttpOboAgent(ctx, nil)
	if errSt != nil {
		return nil, errSt.Err()
	}

	err := agent.DeleteBucket(ctx, &cbmgmtx.DeleteBucketOptions{
		OnBehalfOf: oboInfo,
		BucketName: in.BucketName,
	})
	if err != nil {
		if errors.Is(err, cbmgmtx.ErrBucketNotFound) {
			return nil, s.errorHandler.NewBucketMissingStatus(err, in.BucketName).Err()
		}
		return nil, s.errorHandler.NewGenericStatus(err).Err()
	}

	return &admin_bucket_v1.DeleteBucketResponse{}, nil
}
