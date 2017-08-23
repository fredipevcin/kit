package jsonrpc_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/transport/http/jsonrpc"
)

func addBody() io.Reader {
	return strings.NewReader(`{"jsonrpc": "2.0", "method": "add", "params": [3, 2], "id": 1}`)
}

func expectErrorCode(t *testing.T, want int, body []byte) {
	var r jsonrpc.Response
	err := json.Unmarshal(body, &r)
	if err != nil {
		t.Fatalf("Cant' decode response. err=%s, body=%s", err, body)
	}
	if r.Error == nil {
		t.Fatalf("Expected error on response. Got none: %s", body)
	}
	if have := r.Error.Code; want != have {
		t.Fatalf("Unexpected error code. Want %d, have %d: %s", want, have, body)
	}
}

func nopDecoder(context.Context, json.RawMessage) (interface{}, error) { return struct{}{}, nil }
func nopEncoder(context.Context, interface{}) (json.RawMessage, error) { return []byte("[]"), nil }

type mockLogger struct {
	Called   bool
	LastArgs []interface{}
}

func (l *mockLogger) Log(keyvals ...interface{}) error {
	l.Called = true
	l.LastArgs = append(l.LastArgs, keyvals)
	return nil
}

func TestServerBadDecode(t *testing.T) {
	logger := mockLogger{}
	sm := jsonrpc.ServiceMap{
		"add": jsonrpc.NewService(
			endpoint.Nop,
			func(context.Context, json.RawMessage) (interface{}, error) { return struct{}{}, errors.New("oof") },
			nopEncoder,
			jsonrpc.ServiceErrorLogger(&logger),
		),
	}
	handler := jsonrpc.NewServer(sm)
	server := httptest.NewServer(handler)
	defer server.Close()
	resp, _ := http.Post(server.URL, "application/json", addBody())
	buf, _ := ioutil.ReadAll(resp.Body)
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		t.Errorf("want %d, have %d: %s", want, have, buf)
	}
	expectErrorCode(t, jsonrpc.InternalError, buf)
	if !logger.Called {
		t.Fatal("Expected logger to be called with error. Wasn't.")
	}
}

func TestServerBadEndpoint(t *testing.T) {
	sm := jsonrpc.ServiceMap{
		"add": jsonrpc.NewService(
			func(context.Context, interface{}) (interface{}, error) { return struct{}{}, errors.New("oof") },
			nopDecoder,
			nopEncoder,
		),
	}
	handler := jsonrpc.NewServer(sm)
	server := httptest.NewServer(handler)
	defer server.Close()
	resp, _ := http.Post(server.URL, "application/json", addBody())
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		t.Errorf("want %d, have %d", want, have)
	}
	buf, _ := ioutil.ReadAll(resp.Body)
	expectErrorCode(t, jsonrpc.InternalError, buf)
}

func TestServerBadEncode(t *testing.T) {
	sm := jsonrpc.ServiceMap{
		"add": jsonrpc.NewService(
			endpoint.Nop,
			nopDecoder,
			func(context.Context, interface{}) (json.RawMessage, error) { return []byte{}, errors.New("oof") },
		),
	}
	handler := jsonrpc.NewServer(sm)
	server := httptest.NewServer(handler)
	defer server.Close()
	resp, _ := http.Post(server.URL, "application/json", addBody())
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		t.Errorf("want %d, have %d", want, have)
	}
	buf, _ := ioutil.ReadAll(resp.Body)
	expectErrorCode(t, jsonrpc.InternalError, buf)
}

func TestServerErrorEncoder(t *testing.T) {
	errTeapot := errors.New("teapot")
	code := func(err error) int {
		if err == errTeapot {
			return http.StatusTeapot
		}
		return http.StatusInternalServerError
	}
	sm := jsonrpc.ServiceMap{
		"add": jsonrpc.NewService(
			func(context.Context, interface{}) (interface{}, error) { return struct{}{}, errTeapot },
			nopDecoder,
			nopEncoder,
		),
	}
	handler := jsonrpc.NewServer(
		sm,
		jsonrpc.ServerErrorEncoder(func(_ context.Context, err error, w http.ResponseWriter) { w.WriteHeader(code(err)) }),
	)
	server := httptest.NewServer(handler)
	defer server.Close()
	resp, _ := http.Post(server.URL, "application/json", addBody())
	if want, have := http.StatusTeapot, resp.StatusCode; want != have {
		t.Errorf("want %d, have %d", want, have)
	}
}

func TestServerUnregisteredMethod(t *testing.T) {
	sm := jsonrpc.ServiceMap{}
	handler := jsonrpc.NewServer(sm)
	server := httptest.NewServer(handler)
	defer server.Close()
	resp, _ := http.Post(server.URL, "application/json", addBody())
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		t.Errorf("want %d, have %d", want, have)
	}
	buf, _ := ioutil.ReadAll(resp.Body)
	expectErrorCode(t, jsonrpc.MethodNotFoundError, buf)
}

func TestServerHappyPath(t *testing.T) {
	step, response := testServer(t)
	step()
	resp := <-response
	defer resp.Body.Close() // nolint
	buf, _ := ioutil.ReadAll(resp.Body)
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		t.Errorf("want %d, have %d (%s)", want, have, buf)
	}
	var r jsonrpc.Response
	err := json.Unmarshal(buf, &r)
	if err != nil {
		t.Fatalf("Cant' decode response. err=%s, body=%s", err, buf)
	}
	if r.Error != nil {
		t.Fatalf("Unxpected error on response: %s", buf)
	}
}

func TestMultipleServerBefore(t *testing.T) {
	var done = make(chan struct{})
	sm := jsonrpc.ServiceMap{
		"add": jsonrpc.NewService(
			endpoint.Nop,
			nopDecoder,
			nopEncoder,
			jsonrpc.ServiceBefore(func(ctx context.Context, _ http.Header) context.Context {
				ctx = context.WithValue(ctx, "one", 1)

				return ctx
			}),
			jsonrpc.ServiceBefore(func(ctx context.Context, _ http.Header) context.Context {
				if _, ok := ctx.Value("one").(int); !ok {
					t.Error("Value was not set properly when multiple ServerBefores are used")
				}

				close(done)
				return ctx
			}),
		),
	}
	handler := jsonrpc.NewServer(sm)
	server := httptest.NewServer(handler)
	defer server.Close()
	http.Post(server.URL, "application/json", addBody()) // nolint

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for finalizer")
	}
}

func TestMultipleServerAfter(t *testing.T) {
	var done = make(chan struct{})
	sm := jsonrpc.ServiceMap{
		"add": jsonrpc.NewService(
			endpoint.Nop,
			nopDecoder,
			nopEncoder,
			jsonrpc.ServiceAfter(func(ctx context.Context, _ http.Header) context.Context {
				ctx = context.WithValue(ctx, "one", 1)

				return ctx
			}),
			jsonrpc.ServiceAfter(func(ctx context.Context, _ http.Header) context.Context {
				if _, ok := ctx.Value("one").(int); !ok {
					t.Error("Value was not set properly when multiple ServerAfters are used")
				}

				close(done)
				return ctx
			}),
		),
	}
	handler := jsonrpc.NewServer(sm)
	server := httptest.NewServer(handler)
	defer server.Close()
	http.Post(server.URL, "application/json", addBody()) // nolint

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for finalizer")
	}
}

func testServer(t *testing.T) (step func(), resp <-chan *http.Response) {
	var (
		stepch   = make(chan bool)
		endpoint = func(ctx context.Context, request interface{}) (response interface{}, err error) {
			<-stepch
			return struct{}{}, nil
		}
		response = make(chan *http.Response)
		sm       = jsonrpc.ServiceMap{
			"add": jsonrpc.NewService(
				endpoint,
				nopDecoder,
				nopEncoder,
			),
		}
		handler = jsonrpc.NewServer(sm)
	)
	go func() {
		server := httptest.NewServer(handler)
		defer server.Close()
		rb := strings.NewReader(`{"jsonrpc": "2.0", "method": "add", "params": [3, 2], "id": 1}`)
		resp, err := http.Post(server.URL, "application/json", rb)
		if err != nil {
			t.Error(err)
			return
		}
		response <- resp
	}()
	return func() { stepch <- true }, response
}
