package scannerdefinitions

import (
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
	"github.com/stackrox/rox/pkg/httputil"
	"github.com/stackrox/rox/pkg/set"
	"github.com/stackrox/rox/pkg/utils"
	mtls2 "github.com/stackrox/scanner/pkg/mtls"
	"google.golang.org/grpc/codes"
)

// scannerDefinitionsHandler handle requests to retrieve scanner vulnerability
// definitions from Central.
type scannerDefinitionsHandler struct {
	http.Handler
	centralClient *http.Client
	centralHost   string
}

// NewDefinitionsHandler creates a new scanner definitions handler.
func NewDefinitionsHandler(centralHost string) (http.Handler, error) {
	// Set up an HTTP client for Central.
	clientConfig, err := mtls2.TLSClientConfigForCentral()
	if err != nil {
		return nil, errors.Wrap(err, "generating TLS client config for Central")
	}
	client := &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			TLSClientConfig: clientConfig,
			// Values are taken from http.DefaultTransport, Go 1.17.3
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	return &scannerDefinitionsHandler{
		centralClient: client,
		centralHost:   centralHost,
	}, nil
}

func (h *scannerDefinitionsHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// Validate request.
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// Prepare the Central's request, proxy relevant headers and all parameters.
	centralURL := url.URL{
		Scheme:   "https",
		Host:     h.centralHost,
		Path:     "api/extensions/scannerdefinitions",
		RawQuery: request.URL.Query().Encode(),
	}
	centralRequest, err := http.NewRequestWithContext(
		request.Context(), http.MethodGet, centralURL.String(), nil)
	if err != nil {
		httputil.WriteGRPCStyleErrorf(writer, codes.Internal, "failed to create request: %v", err)
		return
	}
	// Proxy relevant headers.
	headersToProxy := set.NewFrozenStringSet("If-Modified-Since")
	for _, headerName := range headersToProxy.AsSlice() {
		centralRequest.Header.Set(headerName, request.Header.Get(headerName))
	}
	// Do request, copy all response headers, and body.
	resp, err := h.centralClient.Do(centralRequest)
	if err != nil {
		httputil.WriteGRPCStyleErrorf(writer, codes.Internal, "failed to contact central: %v", err)
		return
	}
	defer utils.IgnoreError(resp.Body.Close)
	for k, vs := range resp.Header {
		for _, v := range vs {
			writer.Header().Set(k, v)
		}
	}
	writer.WriteHeader(resp.StatusCode)
	_, err = io.Copy(writer, resp.Body)
	if err != nil {
		httputil.WriteGRPCStyleErrorf(writer, codes.Internal, "failed write response: %v", err)
		return
	}
}
