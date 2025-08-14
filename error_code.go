package kcp2k

// ErrorCode represents kcp specific error codes to allow for error switching, localization,
// translation to Mirror errors, etc.
type ErrorCode byte

const (
	ErrorCodeDnsResolve      ErrorCode = iota // failed to resolve a host name
	ErrorCodeTimeout                          // ping timeout or dead link
	ErrorCodeCongestion                       // more messages than transport / network can process
	ErrorCodeInvalidReceive                   // recv invalid packet (possibly intentional attack)
	ErrorCodeInvalidSend                      // user tried to send invalid data
	ErrorCodeConnectionClosed                 // connection closed voluntarily or lost involuntarily
	ErrorCodeUnexpected                       // unexpected error / exception, requires fix.
)

// String returns the string representation of the error code
func (e ErrorCode) String() string {
	switch e {
	case ErrorCodeDnsResolve:
		return "DnsResolve"
	case ErrorCodeTimeout:
		return "Timeout"
	case ErrorCodeCongestion:
		return "Congestion"
	case ErrorCodeInvalidReceive:
		return "InvalidReceive"
	case ErrorCodeInvalidSend:
		return "InvalidSend"
	case ErrorCodeConnectionClosed:
		return "ConnectionClosed"
	case ErrorCodeUnexpected:
		return "Unexpected"
	default:
		return "Unknown"
	}
}