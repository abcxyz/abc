package testutil

import (
	"io"
	"strings"
	"testing"
	"time"
)

// ReadWithTimeout does a single read from the given reader. It calls Fatal if
// that read fails or the returned string doesn't contain wantSubStr. May leak a
// goroutine on timeout.
func ReadWithTimeout(tb testing.TB, r io.Reader, wantSubstr string) {
	tb.Helper()

	tb.Logf("readWith starting with %q", wantSubstr)

	var got string
	errCh := make(chan error)
	go func() {
		defer close(errCh)
		buf := make([]byte, 64*1_000)
		tb.Log("to Read")
		n, err := r.Read(buf)
		tb.Log("from Read")
		if err != nil {
			errCh <- err
			return
		}
		got = string(buf[:n])
	}()

	select {
	case err := <-errCh:
		if err != nil {
			tb.Fatal(err)
		}
	case <-time.After(time.Second):
		tb.Fatalf("timed out waiting to read %q", wantSubstr)
	}

	if !strings.Contains(got, wantSubstr) {
		tb.Fatalf("got a prompt %q, but wanted a prompt containing %q", got, wantSubstr)
	}
}

// WriteWithTimeout does a single write to the given writer. It calls Fatal
// if that read doesn't contain wantSubStr. May leak a goroutine on timeout.
func WriteWithTimeout(tb testing.TB, w io.Writer, msg string) {
	tb.Helper()

	tb.Logf("WriteWithTimeout starting with %q", msg)

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		tb.Log("to Write")
		_, err := w.Write([]byte(msg))
		tb.Log("from Write")
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			tb.Fatal(err)
		}
	case <-time.After(time.Second):
		tb.Fatalf("timed out waiting to write %q", msg)
	}
}

type DialogStep struct {
	WaitForPrompt string
	ThenRespond   string // should end with newline
}
