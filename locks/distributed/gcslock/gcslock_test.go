package gcslock

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/dylanplecki/contextlock"
	"github.com/dylanplecki/contextlock/internal/contextlocktest"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

func TestGCSLock(t *testing.T) {
	t.Parallel()

	testBucketName := "test-bucket"
	gcsMockServer := newMockStorageServer(testBucketName)
	gcsMockServerURL, err := url.Parse(gcsMockServer.URL)
	require.NoError(t, err)

	lockFactory := func(t *testing.T) contextlock.ContextLocker {
		lock, err := NewGCSLock(
			testBucketName, fmt.Sprintf("test/%s/object.lock", t.Name()),
			WithLogger(zap.NewNop()),
			WithBaseURL(*gcsMockServerURL),
			WitHTTPClient(gcsMockServer.Client()),
		)
		require.NoError(t, err)
		return lock
	}

	contextlocktest.RunContextLockTestSuite(t, context.Background(), lockFactory)
}

func TestGCSLock_ForceUnlockContext(t *testing.T) {
	t.Parallel()

	testBucketName := "test-bucket"
	testObjectName := fmt.Sprintf("test/%s/object.lock", t.Name())

	gcsMockServer := newMockStorageServer(testBucketName)
	gcsMockServerURL, err := url.Parse(gcsMockServer.URL)
	require.NoError(t, err)

	lockA, err := NewGCSLock(
		testBucketName, testObjectName,
		WithLogger(zap.NewNop()),
		WithBaseURL(*gcsMockServerURL),
		WitHTTPClient(gcsMockServer.Client()),
	)
	require.NoError(t, err)

	lockB, err := NewGCSLock(
		testBucketName, testObjectName,
		WithLogger(zap.NewNop()),
		WithBaseURL(*gcsMockServerURL),
		WitHTTPClient(gcsMockServer.Client()),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// First, have lockA acquire the shared lock object.
	require.NoError(t, lockA.LockContext(ctx))

	// lockB should not be able to release the lock since it didn't acquire it.
	require.Error(t, lockB.UnlockContext(ctx))

	// But lockB can release the lock forcefully even if lockA acquired it.
	require.NoError(t, lockB.ForceUnlockContext(ctx))
}

func TestGCSLock_ForceUnlockContextError(t *testing.T) {
	t.Parallel()

	testBucketName := "test-bucket"
	testObjectName := fmt.Sprintf("test/%s/object.lock", t.Name())

	gcsMockServer := newMockStorageServer(testBucketName)
	gcsMockServerURL, err := url.Parse(gcsMockServer.URL)
	require.NoError(t, err)

	lock, err := NewGCSLock(
		testBucketName, testObjectName,
		WithLogger(zap.NewNop()),
		WithBaseURL(*gcsMockServerURL),
		WitHTTPClient(gcsMockServer.Client()),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Expect a context timeout error.
	require.Error(t, lock.ForceUnlockContext(ctx))
}

func TestGCSLock_NotInitialized(t *testing.T) {
	t.Parallel()

	// Uninitialized GCS lock (invalid).
	var lock GCSLock

	// Attempt without context.
	require.Panicsf(t, func() { lock.Lock() }, "Lock should panic when uninitialized")
	require.Panicsf(t, func() { lock.Unlock() }, "Unlock should panic when uninitialized")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Attempt with context.
	require.Error(t, lock.LockContext(ctx), _gcsLockUninitializedError)
	require.Error(t, lock.UnlockContext(ctx), _gcsLockUninitializedError)
	require.Error(t, lock.ForceUnlockContext(ctx), _gcsLockUninitializedError)
}

func TestGCSLock_CustomBackoff(t *testing.T) {
	t.Parallel()

	testBucketName := "test-bucket"
	testObjectName := fmt.Sprintf("test/%s/object.lock", t.Name())

	gcsMockServer := newMockStorageServer(testBucketName)
	gcsMockServerURL, err := url.Parse(gcsMockServer.URL)
	require.NoError(t, err)

	// Force the mock server to return an error.
	gcsMockServer.SetForceError(errors.New("uh-oh, it looks like we're having problems!"))

	var backOffGenerator BackOffGenerator = func() backoff.BackOff {
		return &backoff.ConstantBackOff{Interval: time.Second}
	}

	lock, err := NewGCSLock(
		testBucketName, testObjectName,
		WithLogger(zap.NewNop()),
		WithBaseURL(*gcsMockServerURL),
		WitHTTPClient(gcsMockServer.Client()),
		WithBackOffGenerator(backOffGenerator),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	require.Error(t, lock.LockContext(ctx))
	assert.Equal(t, uint64(1), gcsMockServer.GetCallCount())
}

func TestGCSLock_CustomLockFileMetadata(t *testing.T) {
	t.Parallel()

	testBucketName := "test-bucket"
	testObjectName := fmt.Sprintf("test/%s/object.lock", t.Name())

	gcsMockServer := newMockStorageServer(testBucketName)
	gcsMockServerURL, err := url.Parse(gcsMockServer.URL)
	require.NoError(t, err)

	testLockFileData := []byte("my custom lockfile data")
	var lockFileMetadataGenerator LockFileMetadataGenerator = func(context.Context) ([]byte, error) {
		return testLockFileData, nil
	}

	lock, err := NewGCSLock(
		testBucketName, testObjectName,
		WithLogger(zap.NewNop()),
		WithBaseURL(*gcsMockServerURL),
		WitHTTPClient(gcsMockServer.Client()),
		WithLockFileMetadataGenerator(lockFileMetadataGenerator),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	require.NoError(t, lock.LockContext(ctx))
	defer func() { require.NoError(t, lock.UnlockContext(ctx)) }()

	require.Equal(t, testLockFileData, gcsMockServer.buckets[testBucketName][testObjectName])
}

func TestGCSLock_CustomLockFileMetadataError(t *testing.T) {
	t.Parallel()

	testBucketName := "test-bucket"
	testObjectName := fmt.Sprintf("test/%s/object.lock", t.Name())

	gcsMockServer := newMockStorageServer(testBucketName)
	gcsMockServerURL, err := url.Parse(gcsMockServer.URL)
	require.NoError(t, err)

	var lockFileMetadataGenerator LockFileMetadataGenerator = func(context.Context) ([]byte, error) {
		return nil, errors.New("my lockfile generator error")
	}

	lock, err := NewGCSLock(
		testBucketName, testObjectName,
		WithLogger(zap.NewNop()),
		WithBaseURL(*gcsMockServerURL),
		WitHTTPClient(gcsMockServer.Client()),
		WithLockFileMetadataGenerator(lockFileMetadataGenerator),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := lock.LockContext(ctx); assert.Error(t, err) {
		assert.Contains(t, err.Error(), "my lockfile generator error")
	}
}

type mockStorageServer struct {
	*httptest.Server
	*mux.Router

	buckets     map[string]mockStorageBucket
	bucketsLock sync.Mutex

	forceError  error
	callCounter atomic.Uint64
}

type mockStorageBucket map[string][]byte

var _ http.Handler = (*mockStorageServer)(nil)

func newMockStorageServer(buckets ...string) *mockStorageServer {
	s := &mockStorageServer{
		buckets: make(map[string]mockStorageBucket),
		Router:  mux.NewRouter(),
	}

	for _, bucketName := range buckets {
		s.buckets[bucketName] = make(mockStorageBucket)
	}

	s.Router.
		Path("/upload/storage/v1/b/{bucketName}/o").
		Methods(http.MethodPost).
		Queries("name", "{objectName}", "uploadType", "media", "ifGenerationMatch", "0").
		HandlerFunc(s.serveObject)

	s.Router.
		Path("/storage/v1/b/{bucketName}/o/{objectName}").
		Methods(http.MethodDelete).
		HandlerFunc(s.serveObject)

	s.Server = httptest.NewServer(s.Router)
	return s
}

func (s *mockStorageServer) SetForceError(err error) {
	s.forceError = err
}

func (s *mockStorageServer) GetCallCount() uint64 {
	return s.callCounter.Load()
}

func (s *mockStorageServer) serveObject(w http.ResponseWriter, req *http.Request) {
	s.callCounter.Inc()

	if s.forceError != nil {
		http.Error(w, s.forceError.Error(), http.StatusInternalServerError)
		return
	}

	bucketName, err := url.PathUnescape(mux.Vars(req)["bucketName"])
	if err != nil {
		http.Error(w,
			errors.Errorf("invalid bucketName %q", mux.Vars(req)["bucketName"]).Error(),
			http.StatusBadRequest,
		)
		return
	}

	objectName, err := url.PathUnescape(mux.Vars(req)["objectName"])
	if err != nil {
		http.Error(w,
			errors.Errorf("invalid objectName %q", mux.Vars(req)["objectName"]).Error(),
			http.StatusBadRequest,
		)
		return
	}

	switch req.Method {
	case http.MethodPost:
		if err, statusCode := s.CreateObject(bucketName, objectName, req.Body); err != nil {
			http.Error(w,
				errors.Wrapf(err, "failed to create object %s:%s", bucketName, objectName).Error(),
				statusCode,
			)
			return
		}

	case http.MethodDelete:
		if err, statusCode := s.DeleteObject(bucketName, objectName); err != nil {
			http.Error(w,
				errors.Wrapf(err, "failed to delete object %s:%s", bucketName, objectName).Error(),
				statusCode,
			)
			return
		}

	default:
		http.Error(w, errors.Errorf("invalid request method %q", req.Method).Error(), http.StatusBadRequest)
		return
	}
}

func (s *mockStorageServer) getBucketUnsafe(bucketName string) (map[string][]byte, error, int) {
	bucket, bucketExists := s.buckets[bucketName]
	if !bucketExists {
		return nil, errors.Errorf("bucket %q does not exist", bucketName), http.StatusNotFound
	}
	return bucket, nil, 0
}

func (s *mockStorageServer) CreateObject(bucketName string, objectName string, objectData io.Reader) (error, int) {
	s.bucketsLock.Lock()
	defer s.bucketsLock.Unlock()

	bucket, err, bucketStatusCode := s.getBucketUnsafe(bucketName)
	if err != nil {
		return err, bucketStatusCode
	}

	if _, exists := bucket[objectName]; exists {
		return errors.Errorf("object %q already exists", objectName), http.StatusBadRequest
	}

	if bucket[objectName], err = ioutil.ReadAll(objectData); err != nil {
		return errors.Wrap(err, "failed to read object data from request body"), http.StatusInternalServerError
	}

	return nil, 0
}

func (s *mockStorageServer) DeleteObject(bucketName string, objectName string) (error, int) {
	s.bucketsLock.Lock()
	defer s.bucketsLock.Unlock()

	bucket, err, bucketStatusCode := s.getBucketUnsafe(bucketName)
	if err != nil {
		return err, bucketStatusCode
	}

	if _, exists := bucket[objectName]; !exists {
		return errors.Errorf("object %q doesn't exist", objectName), http.StatusNotFound
	}

	delete(bucket, objectName)
	return nil, 0
}
