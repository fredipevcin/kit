package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	httptransport "github.com/go-kit/kit/transport/http"
)

type Handler interface {
	ServeJSONRPC(ctx context.Context, params json.RawMessage) (interface{}, http.Header, error)
}

type ServiceMap map[string]Handler

type RequestFunc func(context.Context, http.Header) context.Context

type ServiceResponseFunc func(context.Context, http.Header) context.Context

// ServerOption sets an optional parameter for servers.
type ServerOption func(*Server)

// ServerErrorEncoder is used to encode errors to the http.ResponseWriter
// whenever they're encountered in the processing of a request. Clients can
// use this to provide custom error formatting and response codes. By default,
// errors will be written with the DefaultErrorEncoder.
func ServerErrorEncoder(ee httptransport.ErrorEncoder) ServerOption {
	return func(s *Server) { s.errorEncoder = ee }
}

func NewServer(
	sm ServiceMap,
	options ...ServerOption,
) *Server {
	s := &Server{
		sm:           sm,
		errorEncoder: DefaultErrorEncoder,
	}
	for _, option := range options {
		option(s)
	}
	return s
}

type Server struct {
	sm           ServiceMap
	errorEncoder httptransport.ErrorEncoder
}

// ServeHTTP implements http.Handler.
func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, "405 must POST\n")
		return
	}

	ctx := r.Context()
	ctx = HeaderNewContext(ctx, r.Header)

	// Decode the body into an  object
	var req Request
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		s.errorEncoder(ctx, invalidRequestError{}, w)
		return
	}

	// Get the endpoint and codecs from the map using the method
	// defined in the JSON  object
	srv, ok := s.sm[req.Method]
	if !ok {
		err := methodNotFoundError(fmt.Sprintf("Method %s was not found.", req.Method))
		s.errorEncoder(ctx, err, w)
		return
	}

	resp, respHeaders, err := srv.ServeJSONRPC(ctx, req.Params)
	if err != nil {
		s.errorEncoder(ctx, err, w)
		return
	}

	res := Response{
		RespHeaders: respHeaders,
		JSONRPC:     Version,
	}

	res.Result = resp

	httptransport.EncodeJSONResponse(ctx, w, res)
	// err = httptransport.EncodeJSONResponse(ctx, w, res)
	// if err != nil {
	// 	s.errorEncoder(ctx, err, w)
	// 	return
	// }
}

// ServerFinalizerFunc can be used to perform work at the end of an HTTP
// request, after the response has been written to the client. The principal
// intended use is for request logging. In addition to the response code
// provided in the function signature, additional response parameters are
// provided in the context under keys with the ContextKeyResponse prefix.
type ServerFinalizerFunc func(ctx context.Context, code int, r *http.Request)

// DefaultErrorEncoder writes the error to the ResponseWriter,
// as a json-rpc error response, with an InternalError status code.
// The Error() string of the error will be used as the response error message.
// If the error implements ErrorCoder, the provided code will be set on the
// response error.
// If the error implements Headerer, the given headers will be set.
func DefaultErrorEncoder(_ context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", ContentType)
	if headerer, ok := err.(httptransport.Headerer); ok {
		for k := range headerer.Headers() {
			w.Header().Set(k, headerer.Headers().Get(k))
		}
	}

	e := Error{
		Code:    InternalError,
		Message: err.Error(),
	}
	if sc, ok := err.(ErrorCoder); ok {
		e.Code = sc.ErrorCode()
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(Response{
		JSONRPC: Version,
		Error:   &e,
	})
}

// ErrorCoder is checked by DefaultErrorEncoder. If an error value implements
// ErrorCoder, the Error will be used when encoding the error. By default,
// InternalError (-32603) is used.
type ErrorCoder interface {
	ErrorCode() int
}

// interceptingWriter intercepts calls to WriteHeader, so that a finalizer
// can be given the correct status code.
type interceptingWriter struct {
	http.ResponseWriter
	code int
}

// WriteHeader may not be explicitly called, so care must be taken to
// initialize w.code to its default value of http.StatusOK.
func (w *interceptingWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// StatusCoder is checked by DefaultErrorEncoder. If an error value implements
// StatusCoder, the StatusCode will be used when encoding the error. By default,
// StatusInternalServerError (500) is used.
type StatusCoder interface {
	StatusCode() int
}

// Headerer is checked by DefaultErrorEncoder. If an error value implements
// Headerer, the provided headers will be applied to the response writer, after
// the Content-Type is set.
type Headerer interface {
	Headers() http.Header
}
