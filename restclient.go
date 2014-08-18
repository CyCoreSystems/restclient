/*
	restClient - convenience library fro REST API calls based on JSON

	- Provides a generic interface for any http method
	- Provides transparent JSON marshaling and unmarshaling (assuming appropriately-tagged structs)
	- Support for Basic authentication
	- Support for request timeouts (default: 2s)
	- Classifies status codes and returns appropriate error type
	- Returns http.Status and http.StatusType with error

	Both request and response bodies are expected to be pointers to
	Golang structs with JSON tags.

*/
package restclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/golang/glog"
)

// Auth struct to contain username and password for authentication
type Auth struct {
	Username string
	Password string
}

// Request structures a REST request and provides convenience
// methods for making REST API calls
type Request struct {
	Method string // HTTP Method to use (GET,POST,PUT,DELETE,etc.)
	Url    string // URL to dial (as expected by net.Dial)
	Auth   Auth   // Structure for username and password authentication

	QueryParameters map[string]string // Parameters to attach to the QueryString
	RequestBody     interface{}       // The body of the request
	ResponseBody    interface{}       // The body of the response

	RequestReader io.Reader // Reader interface to the encoded body
	ResponseRaw   []byte    // Raw (usually JSON-encoded) response body

	Timeout time.Duration // Maximum time to wait for response

	Client   http.Client    // Raw http.Client object
	Request  *http.Request  // Raw http.Request object
	Response *http.Response // Raw http.Response object
}

func NewRequest(method string, url string, auth Auth) Request {
	req := Request{Method: method, Url: url, Auth: auth}

	// Set default timeout
	req.Timeout = 2 * time.Second

	// Return new Request
	return req
}

// NewRequest creates a new, basic Request object
func NewRequestBasic(method string, url string) Request {
	auth := Auth{}
	return NewRequest(method, url, auth)
}

// NewRequest creates a new, authenticating Request object
func NewRequestAuth(method string, url string, username string, password string) Request {
	auth := Auth{username, password}
	return NewRequest(method, url, auth)
}

/*
	Do makes a (web) request to the url, populating the 'ret' interface provided,
	and returning the result code from the request

	It is a wrapper around http.Client.Do which JSON-encodes anything going out
	and JSON-decodes anything coming back.

	Basic HTTP response code classifications are performed and the appropriate error
	type are returned.

	In general, this method should not be called directly.
*/
func (r *Request) Do() error {
	glog.V(1).Infoln("Do: started")

	// Encode body to Json from the given body object
	err := r.EncodeRequestBody()
	if err != nil {
		return err
	}

	// Create the client object
	r.createHTTPClient()

	// Create the request object
	err = r.createHTTPRequest()
	if err != nil {
		return err
	}

	// Set the Content-Type header
	r.Request.Header.Add("Content-Type", "application/json")

	// Apply authentication information
	if r.Auth.Username != "" {
		glog.V(2).Infoln("Adding authentication information:", r.Auth)
		r.Request.SetBasicAuth(r.Auth.Username, r.Auth.Password)
	}

	// Send request
	glog.V(2).Infoln("Sending request to server")
	err = r.Execute()
	if err != nil {
		return err
	}

	glog.V(1).Infoln("Do: completed")
	return nil
}

// Execute transacts with the remote server, actually executing
// the Request with the Client.  It sets the Response property on
// successful communication
func (r *Request) Execute() error {
	glog.V(1).Infoln("Execute: started")
	var err error
	r.Response, err = r.Client.Do(r.Request)
	if err != nil {
		glog.Errorln("Failed to make request to server:", err)
		return err
	}
	defer r.Response.Body.Close()

	glog.V(3).Infoln("Server response:", r.Response)

	// Check for error codes
	err = r.ProcessStatusCode()
	if err != nil {
		return err
	}

	// Decode the body
	err = r.DecodeResponse()
	if err != nil {
		return err
	}

	glog.V(1).Infoln("MakeRequest: completed")
	return nil
}

// EncodeRequestBody performs the selected encoding on the
// provided request body, populating the RequestReader
func (r *Request) EncodeRequestBody() error {
	glog.V(1).Infoln("EncodeRequestBody: started")
	// Encode body to Json from the given body object
	if r.RequestBody == nil {
		glog.Warning("Nothing to encode")
		return nil
	}

	glog.V(2).Infoln("Encoding bodyObject (", r.RequestBody, ") to json")
	encodedBytes, err := json.Marshal(r.RequestBody)
	if err != nil {
		return err
	}

	r.RequestReader = bytes.NewReader(encodedBytes)
	glog.V(1).Infoln("EncodeRequestBody: completed")
	return nil
}

// ProcessStatusCode processes and returns classified errors resulting
// from the Response's StatusCode
func (r *Request) ProcessStatusCode() error {
	glog.V(1).Infoln("ProcessStatusCode: started")
	resp := r.Response
	if resp.StatusCode != 200 {
		switch {
		case resp.StatusCode == 404:
			return NotFoundError{resp.StatusCode, resp.Status, fmt.Errorf("%s", resp.Status)}
		case resp.StatusCode >= 400 && resp.StatusCode < 500:
			return RequestError{resp.StatusCode, resp.Status, fmt.Errorf("%s", resp.Status)}
		case resp.StatusCode >= 500 && resp.StatusCode < 600:
			return ServerError{resp.StatusCode, resp.Status, fmt.Errorf("%s", resp.Status)}
		default:
			return fmt.Errorf("Unhandled StatusCode: %s", resp.Status)
		}
	}

	glog.V(1).Infoln("ProcessStatusCode: completed")
	return nil
}

func (r *Request) DecodeResponse() error {
	glog.V(1).Infoln("DecodeResponse: started")

	// Read the body into []byte
	responseJson, err := ioutil.ReadAll(r.Response.Body)
	if err != nil {
		glog.Error("Failed to read from body:", r.Response.Body, err)
		return fmt.Errorf("Failed to read from body:", err)
	}
	glog.V(2).Infoln("Server response:", r.Response.Status, responseJson)

	// Unmarshal into response object
	if len(responseJson) > 0 {
		glog.V(2).Infoln("Decoding response")
		err = json.Unmarshal(responseJson, r.ResponseBody)
		if err != nil {
			glog.Errorln("Failed to decode response body:", responseJson, err)
			return fmt.Errorf("Failed to decode response: %v", err.Error())
		}
	} else {
		glog.V(2).Infoln("Zero-length response body")
	}

	glog.V(1).Infoln("DecodeResponse: completed")
	return nil
}

// createHTTPClient generates the http.Client object
// from default parameters
func (r *Request) createHTTPClient() {
	glog.V(1).Infoln("createHTTPClient: started")

	// Create transport for the request
	glog.V(2).Infoln("Creating http.Transport")
	dial := timeoutDialer(r.Timeout)
	transport := http.Transport{
		Dial: dial,
	}

	// Create Client
	glog.V(2).Infoln("Creating http.Client")
	r.Client = http.Client{
		Transport: &transport,
	}
	glog.V(1).Infoln("createHTTPClient: completed")
}

// createHTTPRequest generates the actual http.Request object
// from default parameters
func (r *Request) createHTTPRequest() error {
	glog.V(1).Infoln("createHTTPRequest: started")
	// Create the new request
	var err error
	r.Request, err = http.NewRequest(r.Method, r.Url, r.RequestReader)
	if err != nil {
		glog.Errorln("Failed to create request:", err)
		return err
	}

	glog.V(1).Infoln("createHTTPRequest: completed")
	return nil
}

// Get is a shorthand for MakeRequest("GET",url,nil,ret)
// It calls the ARI server with a GET request
func Get(url string, auth Auth, ret interface{}) error {
	r := NewRequest("GET", url, auth)
	r.ResponseBody = ret
	return r.Do()
}

// Post is a shorthand for MakeRequest("POST",url,req,ret)
// It calls the ARI server with a POST request
func Post(url string, auth Auth, req interface{}, ret interface{}) error {
	r := NewRequest("POST", url, auth)
	r.RequestBody = req
	r.ResponseBody = ret
	return r.Do()
}

// Put is a shorthand for MakeRequest("PUT",url,req,ret)
// It calls the ARI server with a PUT request
func Put(url string, auth Auth, req interface{}, ret interface{}) error {
	r := NewRequest("PUT", url, auth)
	r.RequestBody = req
	r.ResponseBody = ret
	return r.Do()
}

// Delete is a shorthand for MakeRequest("DELETE",url,nil,nil)
// It calls the ARI server with a DELETE request
func Delete(url string, auth Auth, req interface{}, ret interface{}) error {
	r := NewRequest("DELETE", url, auth)
	r.RequestBody = req
	r.ResponseBody = ret
	return r.Do()
}

/*
 *
 *  Helper functions
 *
 */
// timeoutDialer is a wrapper function which returns a customized
// Dial function with a built-in timer for the provided timeout
// duration
func timeoutDialer(timeout time.Duration) func(network, addr string) (net.Conn, error) {
	return func(network string, address string) (net.Conn, error) {
		return net.DialTimeout(network, address, timeout)
	}
}

/*
 *
 *  ERROR structs
 *
 */
// NotFoundError indicates a 404 status code was received
// from the ARI server
type NotFoundError struct {
	StatusCode int
	Status     string
	Err        error
}

func (e NotFoundError) Error() string {
	return "Request: Not Found: " + e.Err.Error()
}

// RequestError indicates a 4xx-level code other than 404
// was received from the server
type RequestError struct {
	StatusCode int
	Status     string
	Err        error
}

func (e RequestError) Error() string {
	return "Request: Request failed: " + e.Status + ": " + e.Err.Error()
}

// ServerError indicates a 5xx-level code was received from
// the server
type ServerError struct {
	StatusCode int
	Status     string
	Err        error
}

func (e ServerError) Error() string {
	err := "Request: Server failure: " + e.Status + ": " + e.Err.Error()
	return err
}
