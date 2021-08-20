package gcslock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
	"golang.org/x/oauth2/google"
)

// Option provides structured optional arguments to NewGCSLock.
type Option func(*GCSLock)

// WithLogger sets the Zap logger to use with the GCSLock.
func WithLogger(logger *zap.Logger) Option {
	return func(lock *GCSLock) {
		lock.logger = logger
	}
}

// WithBaseURL sets the base URL for the Google Cloud Storage API to use with
// HTTP calls from GCSLock.
func WithBaseURL(baseURL url.URL) Option {
	return func(lock *GCSLock) {
		lock.baseURL = baseURL
	}
}

// BackOffGenerator is used to create new backoff iterators.
type BackOffGenerator func() backoff.BackOff

// WithBackOffGenerator sets the backoff implementation to use with GCSLock.
func WithBackOffGenerator(backOffGenerator BackOffGenerator) Option {
	return func(lock *GCSLock) {
		lock.backOffGenerator = backOffGenerator
	}
}

// DefaultBackOffGenerator provides a default exponential backoff implementation
// for use with WithBackOffGenerator.
func DefaultBackOffGenerator() BackOffGenerator {
	return func() backoff.BackOff {
		return &backoff.ExponentialBackOff{
			InitialInterval:     time.Millisecond,
			RandomizationFactor: 0.05, // +-5% randomization.
			Multiplier:          1.01, // +1% growth per retry, 394 ops (~5s) to max interval.
			MaxInterval:         50 * time.Millisecond,
			MaxElapsedTime:      0, // Never expire.
			Stop:                backoff.Stop,
			Clock:               backoff.SystemClock,
		}
	}
}

// HTTPClientDoer provides an interface wrapper around a *http.Client.
type HTTPClientDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// WitHTTPClient sets the HTTP client to use with GCSLock HTTP calls to the
// Google Cloud Storage API.
func WitHTTPClient(httpClient HTTPClientDoer) Option {
	return func(lock *GCSLock) {
		lock.httpClient = httpClient
	}
}

// DefaultGoogleHTTPClient provides a standard Google API HTTP client to be
// used with WitHTTPClient.
func DefaultGoogleHTTPClient(ctx context.Context, scopes ...string) (HTTPClientDoer, error) {
	return google.DefaultClient(ctx, scopes...)
}

// LockFileMetadataGenerator creates output data to be stored in a GCS lock
// file when created. This may contain useful information about the locker's
// identity and execution path for debugging purposes.
type LockFileMetadataGenerator func(ctx context.Context) ([]byte, error)

// WithLockFileMetadataGenerator sets the lock file metadata generator to be
// used with GCSLock.
func WithLockFileMetadataGenerator(generator LockFileMetadataGenerator) Option {
	return func(lock *GCSLock) {
		lock.lockFileMetadata = generator
	}
}

// DefaultLockFileMetadataGenerator provides a default lock file metadata
// generator including information about the time, host, and call stack of the
// lock acquisition call to be used with WithLockFileMetadataGenerator.
func DefaultLockFileMetadataGenerator() LockFileMetadataGenerator {
	return func(ctx context.Context) ([]byte, error) {
		var metadata struct {
			LockedTime     string
			LockedByHost   *string
			LockStackTrace []byte
		}

		metadata.LockedTime = time.Now().String()

		// Get hostname of the current host.
		if hostname, err := os.Hostname(); err == nil {
			metadata.LockedByHost = &hostname
		}

		// Get stack trace for the current goroutine.
		var stackBuf [4096]byte
		bufSizeWritten := runtime.Stack(stackBuf[:], false /* don't lock the state of the world */)
		if bufSizeWritten > 0 {
			metadata.LockStackTrace = stackBuf[:bufSizeWritten]
		}

		return json.Marshal(metadata)
	}
}
