// Copyright 2023 Abah Joseph. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package httplib

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/time/rate"
)

// AsyncResult is the result of an asynchronous request
type AsyncResult struct {
	Response *http.Response
	Error    error
}

// UnmarshalError is returned when there is an error unmarshaling the response body
type UnmarshalError struct {
	Err error
}

// Error returns the error message
func (e *UnmarshalError) Error() string {
	return fmt.Sprintf("httplib: JSON unmarshaling error: %v", e.Err)
}

// MarshalError is returned when there is an error marshaling the request body
type MarshalError struct {
	Err error
}

// Error returns the error message
func (e *MarshalError) Error() string {
	return fmt.Sprintf("httplib: JSON marshaling error: %v", e.Err)
}

// RequestError returned when there is an error making the request
type RequestError struct {
	Err error
}

// Error returns the error message
func (e *RequestError) Error() string {
	return fmt.Sprintf("httplib: request error: %v", e.Err)
}

// Configure the default transport for connection pooling
var defaultTransport = &http.Transport{
	MaxIdleConns:    10,
	IdleConnTimeout: 30 * time.Second,
}

// RequestBuilder is used to build a request. Always use NewRequestBuilder to create a new instance
type RequestBuilder struct {
	method     string
	url        string
	headers    map[string]string
	body       io.Reader
	timeout    time.Duration
	client     *http.Client
	middleware []Middleware
	limiter    *rate.Limiter
	retries    int
	backoff    BackoffFunc
}

// Middleware is a function that is executed on the response before returning it to the caller
type Middleware func(*http.Request, *http.Response) error

// BackoffFunc is a function that is used to calculate the backoff duration between retries
type BackoffFunc func(retry int) time.Duration

// NewRequestBuilder creates a new request builder
func NewRequestBuilder(method, url string) *RequestBuilder {
	return &RequestBuilder{
		method:  method,
		url:     url,
		headers: make(map[string]string),
		timeout: 30 * time.Second,
		client:  &http.Client{Transport: defaultTransport},
	}
}

// WithHeader adds a header to the request
func (rb *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	rb.headers[key] = value
	return rb
}

// WithJSONBody adds a JSON body to the request
func (rb *RequestBuilder) WithJSONBody(data interface{}) (*RequestBuilder, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, &MarshalError{Err: err}
	}
	rb.body = bytes.NewBuffer(jsonData)
	rb.headers["Content-Type"] = "application/json"
	return rb, nil
}

// WithCustomBody adds a custom body to the request
func (rb *RequestBuilder) WithCustomBody(body io.Reader) *RequestBuilder {
	rb.body = body
	return rb
}

// WithTimeout sets the timeout for the request
func (rb *RequestBuilder) WithTimeout(timeout time.Duration) *RequestBuilder {
	rb.timeout = timeout
	rb.client.Timeout = timeout
	return rb
}

// WithClient sets the http client to use for the request
func (rb *RequestBuilder) WithClient(client *http.Client) *RequestBuilder {
	rb.client = client
	return rb
}

// BodyAsBytes returns the request body as a byte array
func (rb *RequestBuilder) BodyAsBytes() ([]byte, error) {
	return io.ReadAll(rb.body)
}

// Use Add a function to register middleware
func (rb *RequestBuilder) Use(middleware Middleware) *RequestBuilder {
	if rb.retries > 0 && rb.backoff != nil {
		rb.middleware = append(rb.middleware, rb.retryMiddleware)
	}
	rb.middleware = append(rb.middleware, middleware)
	return rb
}

// ReadJSONResponse reads the response body and unmarshals it into the target object
func (rb *RequestBuilder) ReadJSONResponse(response *http.Response, target interface{}) error {
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return &UnmarshalError{Err: err}
	}
	err = json.Unmarshal(body, target)
	if err != nil {
		return &UnmarshalError{Err: err}
	}
	return nil
}

// WithRateLimiter sets the rate limiter for the request
func (rb *RequestBuilder) WithRateLimiter(rps int, burst int) *RequestBuilder {
	rb.limiter = rate.NewLimiter(rate.Limit(rps), burst)
	return rb
}

// WithRetry sets the number of retries and the backoff function for the request
func (rb *RequestBuilder) WithRetry(retries int, backoff BackoffFunc) *RequestBuilder {
	rb.retries = retries
	rb.backoff = backoff
	return rb
}

// retryMiddleware is a middleware that retries the request if the response status code is 5xx
func (rb *RequestBuilder) retryMiddleware(req *http.Request, res *http.Response) error {
	if rb.retries == 0 || res.StatusCode < 500 {
		return nil
	}

	for i := 0; i < rb.retries; i++ {
		time.Sleep(rb.backoff(i))

		resp, err := rb.client.Do(req)
		if err == nil && resp.StatusCode < 500 {
			*res = *resp
			return nil
		}
	}

	return fmt.Errorf("httplib: all retries failed")
}

// Send sends the request and returns the response
func (rb *RequestBuilder) Send() (*http.Response, error) {

	if rb.limiter != nil {
		err := rb.limiter.Wait(context.Background())
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(rb.method, rb.url, rb.body)
	if err != nil {
		return nil, &RequestError{Err: err}
	}

	for key, value := range rb.headers {
		req.Header.Set(key, value)
	}

	response, err := rb.client.Do(req)
	if err != nil {
		return nil, &RequestError{Err: err}
	}

	// Execute middleware functions
	for _, middleware := range rb.middleware {
		err = middleware(req, response)
		if err != nil {
			return nil, err
		}
	}

	return response, nil
}

// SendAndChain sends the request and returns the response
func (rb *RequestBuilder) SendAndChain(chain func(*http.Response) (*RequestBuilder, error)) (*http.Response, error) {
	response, err := rb.Send()
	if err != nil {
		return nil, err
	}

	chainedRequestBuilder, err := chain(response)
	if err != nil {
		return nil, err
	}

	return chainedRequestBuilder.Send()
}

// SendAsync sends the request asynchronously and returns a channel that will contain the response
func (rb *RequestBuilder) SendAsync() <-chan AsyncResult {
	resultChan := make(chan AsyncResult)

	go func() {
		response, err := rb.Send()
		resultChan <- AsyncResult{Response: response, Error: err}
		close(resultChan)
	}()

	return resultChan
}

// ExponentialBackoff calculates the backoff duration for retries using exponential backoff strategy
func ExponentialBackoff(retry int) time.Duration {
    baseDelay := 100 * time.Millisecond
    maxDelay := 10 * time.Second
    factor := 2.0
    jitter := 0.2

    delay := float64(baseDelay) * math.Pow(factor, float64(retry))
    if delay > float64(maxDelay) {
        delay = float64(maxDelay)
    }

    jitterRange := jitter * delay
    delay -= jitterRange / 2
    delay += rand.Float64() * jitterRange

    return time.Duration(delay)
}

// WithFile adds a file to the request as a multipart form data
func (rb *RequestBuilder) WithFile(fieldName, filePath string) (*RequestBuilder, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+filepath.Base(fileInfo.Name())+`"`)
	h.Set("Content-Type", "application/octet-stream")

	part, err := writer.CreatePart(h)
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	rb.body = body
	rb.headers["Content-Type"] = writer.FormDataContentType()

	return rb, nil
}