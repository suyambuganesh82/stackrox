package scannerdefinitions

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

type responseWriterMock struct {
	bytes.Buffer
	statusCode int
	headers    http.Header
}

func NewMockResponseWriter() *responseWriterMock {
	return &responseWriterMock{
		headers: make(http.Header),
	}
}

func (m *responseWriterMock) Header() http.Header {
	return m.headers
}

func (m *responseWriterMock) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

// transportMockFunc is a transport mock that call itself to implement http.Transport's RoundTrip.
type transportMockFunc func(req *http.Request) (*http.Response, error)

func (f transportMockFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func Test_scannerDefinitionsHandler_ServeHTTP(t *testing.T) {
	type args struct {
		writer  *responseWriterMock
		request *http.Request
	}
	tests := []struct {
		name         string
		args         args
		responseBody string
		statusCode   int
	}{
		{
			name:         "when central replies 200 with content then writer matches",
			statusCode:   200,
			responseBody: "the foobar body.",
			args: args{
				writer: NewMockResponseWriter(),
			},
		},
		{
			name:         "when central replies 304 then writer matches",
			statusCode:   304,
			responseBody: "",
			args: args{
				writer: NewMockResponseWriter(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &scannerDefinitionsHandler{
				centralClient: &http.Client{
					Transport: transportMockFunc(func(req *http.Request) (*http.Response, error) {
						assert.Equal(t, "bar=1&foo=2", req.URL.RawQuery)
						assert.Equal(t, []string{"1209"}, req.Header["If-Modified-Since"])
						return &http.Response{
							StatusCode: tt.statusCode,
							Body:       ioutil.NopCloser(bytes.NewBufferString(tt.responseBody)),
						}, nil
					}),
				},
			}
			h.ServeHTTP(tt.args.writer, &http.Request{
				URL:    &url.URL{RawQuery: "bar=1&foo=2"},
				Header: map[string][]string{"If-Modified-Since": []string{"1209"}},
			})
			assert.Equal(t, tt.responseBody, tt.args.writer.String())
			assert.Equal(t, tt.statusCode, tt.args.writer.statusCode)
		})
	}
}
