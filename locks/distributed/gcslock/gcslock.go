package gcslock

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/dylanplecki/contextlock"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

// Error messages.
const (
	_gcsLockUninitializedError = "GCSLock not initialized with NewGCSLock"
)

// Google Cloud scopes.
const (
	_googleAuthScopeDevStorageFullControl = "https://www.googleapis.com/auth/devstorage.full_control"
)

// GCSLock uses the strong-consistency guarantees of Google Cloud Storage (GCS)
// to provide a fault-tolerant, globally-distributed lock via an HTTP API.
//
// It does so by writing a specific object to a specified bucket on lock
// acquisition, retrying (with backoff) if the file already exists or when a
// network error occurs. To release the lock, the same object is deleted.
//
// See https://cloud.google.com/storage/docs/consistency for more details on
// the consistency guarantees provided by GCS.
type GCSLock struct {
	logger *zap.Logger

	bucket  string
	object  string
	baseURL url.URL

	// Use a sync.Mutex struct to prevent GCSLock struct copying.
	_            sync.Mutex
	lockAcquired atomic.Bool

	httpClient       HTTPClientDoer
	backOffGenerator BackOffGenerator
	lockFileMetadata LockFileMetadataGenerator
}

var _ contextlock.ContextLocker = (*GCSLock)(nil)

// NewGCSLock creates a new Google Cloud Storage (GCS) globally-consistent
// contextlock.ContextLocker type, which will write the lock file to the
// provided GCS bucket and object (directory & file).
func NewGCSLock(bucket string, object string, opts ...Option) (*GCSLock, error) {
	l := &GCSLock{
		logger: zap.L(),

		bucket: bucket,
		object: object,
		baseURL: url.URL{
			Scheme: "https",
			Host:   "storage.googleapis.com",
		},

		backOffGenerator: DefaultBackOffGenerator(),
		lockFileMetadata: DefaultLockFileMetadataGenerator(),
	}

	for _, opt := range opts {
		opt(l)
	}

	l.logger = l.logger.With(zap.String("bucket", bucket), zap.String("object", object))

	if l.httpClient == nil {
		httpClient, err := DefaultGoogleHTTPClient(context.Background(), _googleAuthScopeDevStorageFullControl)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create default google http client")
		}
		l.httpClient = httpClient
	}

	return l, nil
}

// Lock implements sync.Locker.
func (l *GCSLock) Lock() {
	if err := l.LockContext(context.Background()); err != nil {
		panic(err)
	}
}

// Unlock implements sync.Locker.
func (l *GCSLock) Unlock() {
	if err := l.UnlockContext(context.Background()); err != nil {
		panic(err)
	}
}

func (l *GCSLock) checkInit() error {
	if l.httpClient == nil {
		return errors.New(_gcsLockUninitializedError)
	}
	return nil
}

// LockContext implements contextlock.ContextLocker.
func (l *GCSLock) LockContext(ctx context.Context) error {
	if err := l.checkInit(); err != nil {
		return err
	}
	return l.writeObject(ctx)
}

// UnlockContext implements contextlock.ContextLocker.
func (l *GCSLock) UnlockContext(ctx context.Context) error {
	if err := l.checkInit(); err != nil {
		return err
	}
	return l.deleteObject(ctx, false /* forceUnlock */)
}

// ForceUnlockContext is a special variant of UnlockContext that will forcefully
// unlock the global state by deleting the GCS object even if the local lock
// instance did not create it.
//
// This method can be useful to reset the global lock state when the process
// that acquired the lock exits abnormally and fails to release its lock.
func (l *GCSLock) ForceUnlockContext(ctx context.Context) error {
	if err := l.checkInit(); err != nil {
		return err
	}
	return l.deleteObject(ctx, true /* forceUnlock */)
}

func (l *GCSLock) writeObject(ctx context.Context) error {
	lockFileMetadata, err := l.lockFileMetadata(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to generate lock file metadata")
	}

	uploadURL := l.baseURL
	uploadURL.Path += fmt.Sprintf("/upload/storage/v1/b/%s/o",
		url.PathEscape(l.bucket),
	)
	uploadQuery := uploadURL.Query()
	uploadQuery.Set("name", l.object)
	uploadQuery.Set("uploadType", "media")
	uploadQuery.Set("ifGenerationMatch", "0")
	uploadURL.RawQuery = uploadQuery.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL.String(), newBytesReader(lockFileMetadata))
	if err != nil {
		return errors.Wrapf(err, "failed to create gcs upload request for lock file URL %q", uploadURL.String())
	}

	if err := l.sendRequestWithRetry(ctx, req); err != nil {
		return errors.Wrapf(err, "gcs object create http request failed")
	}

	l.lockAcquired.Store(true)
	l.logger.Debug("acquired lock")
	return nil
}

func (l *GCSLock) deleteObject(ctx context.Context, forceUnlock bool) error {
	if !forceUnlock && !l.lockAcquired.Load() {
		return errors.New("lock has not been locally acquired before unlock")
	}

	accessURL := l.baseURL
	accessURL.Path += fmt.Sprintf("/storage/v1/b/%s/o/%s",
		url.PathEscape(l.bucket),
		url.PathEscape(l.object),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, accessURL.String(), nil)
	if err != nil {
		return errors.Wrapf(err, "failed to create gcs delete request for lock file URL %q", accessURL.String())
	}

	if err := l.sendRequestWithRetry(ctx, req); err != nil {
		return errors.Wrapf(err, "gcs object delete http request failed")
	}

	l.lockAcquired.Store(false)
	l.logger.Debug("released lock")
	return nil
}

func (l *GCSLock) sendRequestWithRetry(ctx context.Context, request *http.Request) error {
	backOff := l.backOffGenerator()

	for {
		resp, err := l.httpClient.Do(request)

		if err != nil {
			l.logger.Debug(
				"http client request returned an error",
				zap.String("method", request.Method), zap.Error(err),
			)

		} else if resp.StatusCode == 200 {
			// Yay! The HTTP request completed successfully.
			return nil

		} else {
			l.logger.Debug(
				"received non-200 http response status",
				zap.String("method", request.Method), zap.Int("status", resp.StatusCode),
			)
			err = errors.New(resp.Status) // For use with multierr.Combine below.
		}

		select {
		case <-ctx.Done():
			ctxErr := ctx.Err()
			if err != context.Canceled && err != context.DeadlineExceeded {
				multierr.AppendInto(&ctxErr, err) // Retain err from http call when context expires.
			}
			return ctxErr
		case <-time.After(backOff.NextBackOff()):
			// Continue.
		}
	}
}

type bytesReader struct {
	*bytes.Reader
}

var _ io.ReadCloser = (*bytesReader)(nil)

func newBytesReader(b []byte) *bytesReader {
	return &bytesReader{
		Reader: bytes.NewReader(b),
	}
}

// Close will seek the bytes.Reader back to the beginning of the byte slice.
func (r *bytesReader) Close() error {
	_, err := r.Seek(0, io.SeekStart)
	return err
}
