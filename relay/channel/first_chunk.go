package channel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/relaykit/types"
)

var errFirstChunkTimeout = errors.New("upstream first chunk timeout")

// firstChunkBody preserves the byte read before dispatch to a protocol handler.
// Its context must remain alive until the handler closes the response body.
type firstChunkBody struct {
	io.Reader
	body             io.ReadCloser
	cancel           context.CancelCauseFunc
	stopClientCancel func() bool
}

func (b *firstChunkBody) Close() error {
	b.stopClientCancel()
	b.cancel(context.Canceled)
	return b.body.Close()
}

// awaitFirstChunk holds the response before any downstream handler can commit
// headers or emit pings. Only one byte is buffered; SSE framing is irrelevant.
func awaitFirstChunk(clientContext context.Context, req *http.Request, timeout time.Duration, send func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	ctx, cancel := context.WithCancelCause(req.Context())
	stopClientCancel := context.AfterFunc(clientContext, func() { cancel(clientContext.Err()) })
	if clientContext.Err() != nil {
		cancel(clientContext.Err())
	}
	var mu sync.Mutex
	finished := false
	// Cover connection acquisition, DNS, TCP/TLS and upload as well as the
	// response wait. Transparent transport retries share this attempt's budget.
	timer := time.AfterFunc(timeout, func() {
		mu.Lock()
		defer mu.Unlock()
		if !finished {
			finished = true
			cancel(errFirstChunkTimeout)
		}
	})
	resp, err := send(req.WithContext(ctx))
	var first [1]byte
	var n int
	if err == nil && resp != nil {
		n, err = io.ReadFull(resp.Body, first[:])
		if err == io.EOF {
			err = nil // A normal empty response is not a new failure condition.
		}
	}
	mu.Lock()
	finished = true
	timer.Stop()
	cause := context.Cause(ctx)
	mu.Unlock()
	if cause == nil {
		cause = clientContext.Err()
	}
	if cause != nil || err != nil || resp == nil {
		stopClientCancel()
		cancel(context.Canceled)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if clientContext.Err() != nil {
			return nil, clientContext.Err()
		}
		if errors.Is(cause, errFirstChunkTimeout) {
			return nil, types.NewErrorWithStatusCode(errFirstChunkTimeout, types.ErrorCodeUpstreamFirstChunkTimeout, 499)
		}
		if err != nil {
			return nil, err
		}
		if cause != nil {
			return nil, cause
		}
		return nil, errors.New("upstream response is nil")
	}
	resp.Body = &firstChunkBody{
		Reader: io.MultiReader(bytes.NewReader(first[:n]), resp.Body),
		body:   resp.Body, cancel: cancel, stopClientCancel: stopClientCancel,
	}
	return resp, nil
}
