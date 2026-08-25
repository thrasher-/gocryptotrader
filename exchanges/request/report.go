package request

import (
	"time"
)

// Reporter interface groups observability functionality over HTTP request
// latency. The path argument is the raw request path and can contain
// credentials; reporter implementations must treat it as sensitive data.
type Reporter interface {
	Latency(name, method, path string, t time.Duration)
}

// SetupGlobalReporter sets a reporter interface to be used
// for all exchange requests
func SetupGlobalReporter(r Reporter) {
	globalReporter = r
}
