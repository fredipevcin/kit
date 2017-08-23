package jsonrpc

// Error defines a JSON RPC error that can be returned
// in a Response from the spec
// http://www.jsonrpc.org/specification#error_object
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

const (
	// ParseError defines invalid JSON was received by the server.
	// An error occurred on the server while parsing the JSON text.
	ParseError int = -32700

	// InvalidRequestError defines the JSON sent is not a valid Request object.
	InvalidRequestError int = -32600

	// MethodNotFoundError defines the method does not exist / is not available.
	MethodNotFoundError int = -32601

	// InvalidParamsError defines invalid method parameter(s).
	InvalidParamsError int = -32602

	// InternalError defines a server error
	InternalError int = -32603
)

var errorMessage = map[int]string{
	ParseError:          "An error occurred on the server while parsing the JSON text.",
	InvalidRequestError: "The JSON sent is not a valid Request object.",
	MethodNotFoundError: "The method does not exist / is not available.",
	InvalidParamsError:  "Invalid method parameter(s).",
	InternalError:       "Internal JSON-RPC error.",
}

// ErrorMessage returns a message for the JSON RPC error code. It returns the empty
// string if the code is unknown.
func ErrorMessage(code int) string {
	return errorMessage[code]
}

type parseError struct{}

func (e *parseError) ErrorCode() int {
	return ParseError
}

type invalidRequestError struct{}

func (e invalidRequestError) Error() string {
	return ErrorMessage(e.ErrorCode())
}

func (e *invalidRequestError) ErrorCode() int {
	return InvalidRequestError
}

type methodNotFoundError string

func (e methodNotFoundError) Error() string {
	return string(e)
}

func (e methodNotFoundError) ErrorCode() int {
	return MethodNotFoundError
}

type invalidParamsError struct{}

func (e invalidParamsError) ErrorCode() int {
	return InvalidParamsError
}

type internalError struct{}

func (e *internalError) ErrorCode() int {
	return InternalError
}
