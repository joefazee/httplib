// Copyright 2023 Abah Joseph. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package httplib

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func TestNewRequestBuilder(t *testing.T) {
	method := "GET"
	url := "https://api.example.com/data"

	rb := NewRequestBuilder(method, url)
	assert.NotNil(t, rb)

	assert.Equal(t, method, rb.method)
	assert.Equal(t, url, rb.url)
	assert.NotNil(t, rb.headers)
}

func TestWithHeader(t *testing.T) {
	method := "GET"
	url := "https://api.example.com/data"
	headerKey1 := "Content-Type"
	headerValue1 := "application/json"
	headerKey2 := "Authorization"
	headerValue2 := "Bearer your_token_here"
	headerValue2Updated := "Bearer new_token_here"

	rb := NewRequestBuilder(method, url)

	// Add the first header
	rb.WithHeader(headerKey1, headerValue1)

	value, ok := rb.headers[headerKey1]
	assert.True(t, ok, "Expected header %s to be set, but it was not found", headerKey1)
	assert.Equal(t, headerValue1, value, "Expected header %s value to be %s, got %s", headerKey1, headerValue1, value)

	// Add the second header
	rb.WithHeader(headerKey2, headerValue2)
	value, ok = rb.headers[headerKey2]
	assert.True(t, ok, "Expected header %s to be set, but it was not found", headerKey2)
	assert.Equal(t, headerValue2, value, "Expected header %s value to be %s, got %s", headerKey2, headerValue2, value)

	// Update the second header
	rb.WithHeader(headerKey2, headerValue2Updated)
	value, ok = rb.headers[headerKey2]
	assert.True(t, ok, "Expected header %s to be updated, but it was not found", headerKey2)
	assert.Equal(t, headerValue2Updated, value, "Expected header %s value to be %s after update, got %s", headerKey2, headerValue2Updated, value)
}

func TestWithJSONBody(t *testing.T) {
	method := "POST"
	url := "https://api.example.com/data"

	rb := NewRequestBuilder(method, url)

	data := map[string]string{
		"key": "value",
	}

	rb.WithJSONBody(data)

	jsonData, err := json.Marshal(data)
	assert.NoError(t, err, "Error marshaling JSON data: %v", err)
	assert.NotNil(t, rb.body, "JSON body not set correctly")

	b, err := rb.BodyAsBytes()
	assert.NotNil(t, b, "Error getting body as bytes: %v", err)
	assert.True(t, bytes.Equal(b, jsonData), "JSON body not set correctly")
	assert.Equal(t, "application/json", rb.headers["Content-Type"], "Content-Type header not set correctly for JSON body")
}

func TestWithCustomBody(t *testing.T) {
	method := "POST"
	url := "https://api.example.com/data"

	rb := NewRequestBuilder(method, url)

	body := "custom body content"
	contentType := "text/plain"

	rb.WithCustomBody(strings.NewReader(body)).WithHeader("Content-Type", contentType)

	b, err := rb.BodyAsBytes()
	assert.NotNil(t, b, "Error getting body as bytes: %v", err)
	assert.Equal(t, body, string(b), "Custom body not set correctly")
	assert.Equal(t, contentType, rb.headers["Content-Type"], "Content-Type header not set correctly for custom body")

}

func TestWithTimeout(t *testing.T) {
	method := "GET"
	url := "https://api.example.com/data"

	rb := NewRequestBuilder(method, url)

	timeout := 5 * time.Second
	rb.WithTimeout(timeout)

	assert.Equal(t, timeout, rb.client.Timeout, "Expected client timeout to be %v, got %v", timeout, rb.client.Timeout)

}

func TestWithClient(t *testing.T) {
	customClient := &http.Client{
		Timeout: 10 * time.Second,
	}
	rb := NewRequestBuilder("GET", "https://api.example.com/data").WithClient(customClient)
	// use assert
	assert.Equal(t, customClient, rb.client, "Expected custom client to be set, got a different client")

}

func TestSend(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	// Custom backoff function
	backoffFunc := func(retry int) time.Duration {
		return time.Duration(retry) * 100 * time.Millisecond
	}

	rb := NewRequestBuilder("GET", server.URL).
		WithRateLimiter(1, 1).
		WithRetry(2, backoffFunc)

	resp, err := rb.Use(rb.retryMiddleware).Send()

	assert.Nil(t, err, "Expected no error, got %v", err)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	assert.Equal(t, 3, requestCount, "Expected %d requests due to retries, got %d", 3, requestCount)
}

func TestRequestBuilder_SendAndChain(t *testing.T) {
	tests := []struct {
		name                string
		firstHandler        http.HandlerFunc
		secondHandler       http.HandlerFunc
		expectedStatus      int
		expectError         bool
		expectChainingError bool
	}{
		{
			name: "Successful Chained Calls",
			firstHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			},
			secondHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			},
			expectedStatus:      http.StatusOK,
			expectError:         false,
			expectChainingError: false,
		},
		{
			name: "Chaining Error",
			firstHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			},
			secondHandler:       nil, // No handler will cause a chaining error
			expectedStatus:      0,
			expectError:         false,
			expectChainingError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firstServer := httptest.NewServer(tt.firstHandler)
			defer firstServer.Close()

			secondServer := httptest.NewServer(tt.secondHandler)
			defer secondServer.Close()

			reqBuilder := NewRequestBuilder("GET", firstServer.URL)
			resp, err := reqBuilder.SendAndChain(func(response *http.Response) (*RequestBuilder, error) {
				if tt.expectChainingError {
					return nil, errors.New("Chaining error")
				}
				return NewRequestBuilder("GET", secondServer.URL), nil
			})
			if tt.expectChainingError {
				assert.NotNil(t, err, "Expected a chaining error, but got none")
				return
			}
			if tt.expectError {
				assert.NotNil(t, err, "Expected an error, but got none")
				return
			}
			assert.Nil(t, err, "Unexpected error: %v", err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "Expected status code %d, but got %d", tt.expectedStatus, resp.StatusCode)

		})
	}
}

func TestRequestBuilder_ReadJSONResponse(t *testing.T) {
	type TestData struct {
		Message string `json:"message"`
	}

	tests := []struct {
		name         string
		handler      http.HandlerFunc
		expectedData *TestData
		expectError  bool
	}{
		{
			name: "Successful JSON Decoding",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message": "Hello, world!"}`))
			},
			expectedData: &TestData{
				Message: "Hello, world!",
			},
			expectError: false,
		},
		{
			name: "Error Decoding JSON",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message": "Hello, world!",`)) // Invalid JSON
			},
			expectedData: nil,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			reqBuilder := NewRequestBuilder("GET", server.URL)
			resp, err := reqBuilder.Send()
			assert.Nil(t, err, "unexpected error: %v", err)

			var data TestData
			err = reqBuilder.ReadJSONResponse(resp, &data)

			if tt.expectError {
				assert.NotNil(t, err, "expected an error, but got none")
				return
			}

			assert.Nil(t, err, "unexpected error: %v", err)
			assert.True(t, jsonEqual(&data, tt.expectedData), "expected decoded data to be %v, but got %v", tt.expectedData, data)
		})
	}
}

func jsonEqual(a, b interface{}) bool {
	aBytes, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bBytes, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aBytes) == string(bBytes)
}

func TestRequestBuilder_SendAsync(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		expectedError bool
	}{
		{
			name: "Successful Async Request",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			},
			expectedError: false,
		},
		{
			name: "Error Async Request",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Error"))
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			reqBuilder := NewRequestBuilder("GET", server.URL)

			// Use middleware to simulate an error if expectedError is true
			if tt.expectedError {
				reqBuilder.Use(func(req *http.Request, res *http.Response) error {
					return errors.New("some error occurred")
				})
			}

			resultChan := reqBuilder.SendAsync()
			result := <-resultChan

			if tt.expectedError {
				assert.NotNil(t, result.Error, "expected an error, but got none")
			} else {
				assert.Nil(t, result.Error, "unexpected error: %v", result.Error)
				assert.Equal(t, http.StatusOK, result.Response.StatusCode, "expected status code %d, but got %d", http.StatusOK, result.Response.StatusCode)
			}
		})
	}
}

func TestRequestBuilder_Send_EdgeCases(t *testing.T) {
	tests := []struct {
		name                string
		handler             http.HandlerFunc
		configureRequest    func(*RequestBuilder)
		expectedStatusCode  int
		expectedErrContains string
	}{
		{
			name: "http.NewRequest error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			configureRequest: func(rb *RequestBuilder) {
				rb.url = ":" // Invalid URL to cause http.NewRequest to return an error
			},
			expectedStatusCode:  0,
			expectedErrContains: "httplib: request error",
		},
		{
			name: "rb.limiter.Wait error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			configureRequest: func(rb *RequestBuilder) {
				rb.limiter = rate.NewLimiter(rate.Every(10), -1) // Invalid limiter configuration to cause rb.limiter.Wait to return an error
			},
			expectedStatusCode:  0,
			expectedErrContains: "rate: Wait(n=1) exceeds limiter's burst",
		},
		{
			name: "client.Do error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			configureRequest: func(rb *RequestBuilder) {
				rb.client = &http.Client{Transport: errorTransport{}} // Use custom transport that returns an error
			},
			expectedStatusCode:  0,
			expectedErrContains: "httplib: request error",
		},
		{
			name: "headers are added",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Custom-Header") != "HeaderValue" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusOK)
			},
			configureRequest: func(rb *RequestBuilder) {
				rb.WithHeader("Custom-Header", "HeaderValue")
			},
			expectedStatusCode:  http.StatusOK,
			expectedErrContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			reqBuilder := NewRequestBuilder("GET", server.URL)
			tt.configureRequest(reqBuilder)

			response, err := reqBuilder.Send()

			if err != nil {
				if tt.expectedErrContains == "" || !strings.Contains(err.Error(), tt.expectedErrContains) {
					assert.Failf(t, "Unexpected error", "expected: %v, but got: %v", tt.expectedErrContains, err)
				}
			} else {
				assert.Equal(t, tt.expectedStatusCode, response.StatusCode, "unexpected status code")
				assert.Empty(t, tt.expectedErrContains, "unexpected error containing %q", tt.expectedErrContains)
			}
		})
	}
}

type errorTransport struct{}

func (t errorTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, errors.New("custom transport error")
}

func TestUnmarshalError_Error(t *testing.T) {
	err := &UnmarshalError{Err: fmt.Errorf("some error")}
	expected := "httplib: JSON unmarshaling error: some error"
	assert.Equal(t, expected, err.Error())
}

func TestMarshalError_Error(t *testing.T) {
	err := &MarshalError{Err: fmt.Errorf("some error")}
	expected := "httplib: JSON marshaling error: some error"
	assert.Equal(t, expected, err.Error())
}

func TestWithJSONBodyError(t *testing.T) {
	method := "POST"
	url := "https://api.example.com/data"
	rb := NewRequestBuilder(method, url)
	data := make(chan int)
	_, err := rb.WithJSONBody(data)
	assert.NotNil(t, err, "expected an error, but got none")

	_, ok := err.(*MarshalError)
	assert.True(t, ok, "expected a MarshalError, but got %T", err)

}
