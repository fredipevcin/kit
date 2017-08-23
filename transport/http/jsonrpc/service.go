package jsonrpc

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
)

// Service wraps an endpoint and implements http.Handler.
type Service struct {
	e      endpoint.Endpoint
	dec    DecodeRequestFunc
	enc    EncodeResponseFunc
	before []RequestFunc
	after  []ServiceResponseFunc
	logger log.Logger
}

// NewServer constructs a new server, which implements http.Handler and wraps
// the provided endpoint.
func NewService(
	e endpoint.Endpoint,
	dec DecodeRequestFunc,
	enc EncodeResponseFunc,
	options ...ServiceOption,
) *Service {
	s := &Service{
		e:      e,
		dec:    dec,
		enc:    enc,
		logger: log.NewNopLogger(),
	}
	for _, option := range options {
		option(s)
	}
	return s
}

// ServiceOption sets an optional parameter for servers.
type ServiceOption func(*Service)

// ServiceBefore functions are executed on the HTTP request object before the
// request is decoded.
func ServiceBefore(before ...RequestFunc) ServiceOption {
	return func(s *Service) { s.before = append(s.before, before...) }
}

// ServiceAfter functions are executed on the HTTP response writer after the
// endpoint is invoked, but before anything is written to the client.
func ServiceAfter(after ...ServiceResponseFunc) ServiceOption {
	return func(s *Service) { s.after = append(s.after, after...) }
}

// ServiceErrorLogger is used to log non-terminal errors. By default, no errors
// are logged. This is intended as a diagnostic measure. Finer-grained control
// of error handling, including logging in more detail, should be performed in a
// custom ServerErrorEncoder or ServerFinalizer, both of which have access to
// the context.
func ServiceErrorLogger(logger log.Logger) ServiceOption {
	return func(s *Service) { s.logger = logger }
}

// ServeJSONRPC implements Handler
func (s Service) ServeJSONRPC(ctx context.Context, params json.RawMessage) (interface{}, http.Header, error) {

	reqHeaders, ok := HeaderFromContext(ctx)
	if !ok {
		reqHeaders = http.Header{}
	}

	for _, f := range s.before {
		ctx = f(ctx, reqHeaders)
	}

	request, err := s.dec(ctx, params)
	if err != nil {
		s.logger.Log("err", err)
		return nil, nil, err
	}

	response, err := s.e(ctx, request)
	if err != nil {
		s.logger.Log("err", err)
		return nil, nil, err
	}

	respHeaders := http.Header{}
	for _, f := range s.after {
		ctx = f(ctx, respHeaders)
	}

	// Encode the response from the Endpoint
	paramsResp, err := s.enc(ctx, response)
	if err != nil {
		s.logger.Log("err", err)
		return nil, nil, err
	}

	return paramsResp, respHeaders, nil
}
