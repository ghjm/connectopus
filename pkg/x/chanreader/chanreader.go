package chanreader

import (
	"context"
	"io"
)

type ChanReader struct {
	bufsize   int
	errorFunc func(error)
	closeFunc func()
	ch        chan []byte
}

// New returns a ChanReader, which will read from r until EOF or error, sending each read to a channel.  If the
// provided context is cancelled, reads will terminate and the reader will be closed if it is an io.Closer.  These
// behaviors can be overridden by various modifiers.
func New(ctx context.Context, r io.Reader, mods ...func(*ChanReader)) *ChanReader {
	cr := &ChanReader{
		bufsize: 65536,
	}
	cr.closeFunc = func() {
		c, ok := r.(io.Closer)
		if ok {
			_ = c.Close()
		}
	}
	for _, mod := range mods {
		mod(cr)
	}
	cr.ch = make(chan []byte)
	go func() {
		<-ctx.Done()
		cr.closeFunc()
	}()
	go func() {
		defer close(cr.ch)
		for {
			buf := make([]byte, cr.bufsize)
			n, err := r.Read(buf)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				if cr.errorFunc != nil {
					cr.errorFunc(err)
				}
				return
			}
			cr.ch <- buf[:n]
		}
	}()
	return cr
}

func WithBufferSize(bufsize int) func(*ChanReader) {
	return func(cr *ChanReader) {
		cr.bufsize = bufsize
	}
}

func WithErrorFunc(errorFunc func(error)) func(*ChanReader) {
	return func(cr *ChanReader) {
		cr.errorFunc = errorFunc
	}
}

func WithCloseFunc(closeFunc func()) func(*ChanReader) {
	return func(cr *ChanReader) {
		cr.closeFunc = closeFunc
	}
}

func (cr *ChanReader) ReadChan() <-chan []byte {
	return cr.ch
}
