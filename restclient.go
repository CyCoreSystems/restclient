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
	"log"
	"net"
	"net/http"

	"time"
)

// Logger
var Logger *log.Logger

func init() {
	// Null logger, by default
	Logger = log.New(ioutil.Discard, "restclient", log.LstdFlags|log.Lshortfile)
}

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
	RequestType     string            // Request type for request (defaults to "json", options are: "json","form")
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
	Logger.Println("Do: started")

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

	switch r.RequestType {
	case "":
		Logger.Println("No RequestType specified; using json")
		r.Request.Header.Add("Content-Type", "application/json")
	case "json":
		r.Request.Header.Add("Content-Type", "application/json")
	case "form":
		r.Request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	default:
		Logger.Println("Unhandled request type:", r.RequestType)
	}

	// Apply authentication information
	if r.Auth.Username != "" {
		Logger.Printf("Adding authentication information: (%+v)", r.Auth)
		r.Request.SetBasicAuth(r.Auth.Username, r.Auth.Password)
	}

	// Send request
	Logger.Println("Sending request to server")
	err = r.Execute()
	if err != nil {
		return err
	}

	Logger.Println("Do: completed")
	return nil
}

// Execute transacts with the remote server, actually executing
// the Request with the Client.  It sets the Response property on
// successful communication
func (r *Request) Execute() error {
	Logger.Println("Execute: started")
	var err error
	r.Response, err = r.Client.Do(r.Request)
	if err != nil {
		Logger.Println("Failed to make request to server:", err)
		return err
	}
	defer r.Response.Body.Close()

	Logger.Println("Server response:", r.Response)

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

	Logger.Println("MakeRequest: completed")
	return nil
}

// EncodeRequestBody performs the selected encoding on the
// provided request body, populating the RequestReader
func (r *Request) EncodeRequestBody() error {
	Logger.Println("EncodeRequestBody: started")
	// Encode body to Json from the given body object
	if r.RequestBody == nil {
		Logger.Println("Nothing to encode")
		return nil
	}

	// Find encoding type
	if r.RequestType == "" {
		r.RequestType = "json"
	}
	var encodedBytes []byte
	var err error
	switch r.RequestType {
	case "form":
		encodedBytes, err = r.encodeForm()
		if err != nil {
			Logger.Println("Failed to encode form:", err.Error())
			return err
		}
	case "json":
		encodedBytes, err = r.encodeJson()
		if err != nil {
			Logger.Println("Failed to encode form:", err.Error())
			return err
		}
	}

	r.RequestReader = bytes.NewReader(encodedBytes)
	Logger.Println("EncodeRequestBody: completed")
	return nil
}

// encodeJson encodes the request body to Json
func (r *Request) encodeJson() ([]byte, error) {
	Logger.Printf("Encoding bodyObject (%+v) to json", r.RequestBody)
	return json.Marshal(r.RequestBody)
}

// ProcessStatusCode processes and returns classified errors resulting
// from the Response's StatusCode
func (r *Request) ProcessStatusCode() error {
	Logger.Println("ProcessStatusCode: started")
	resp := r.Response
	if (resp.StatusCode >= 300) || (resp.StatusCode < 200) {
		Logger.Printf("Non-2XX response: (%d) %s", resp.StatusCode, resp.Status)
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

	Logger.Println("ProcessStatusCode: completed")
	return nil
}

func (r *Request) DecodeResponse() error {
	Logger.Println("DecodeResponse: started")

	// Read the body into []byte
	responseJson, err := ioutil.ReadAll(r.Response.Body)
	if err != nil {
		Logger.Println("Failed to read from body:", r.Response.Body, err)
		return fmt.Errorf("Failed to read from body:", err)
	}

	// Unmarshal into response object
	if len(responseJson) > 0 {
		Logger.Println("Decoding response")
		err = json.Unmarshal(responseJson, r.ResponseBody)
		if err != nil {
			Logger.Println("Failed to decode response body:", responseJson, err)
			return fmt.Errorf("Failed to decode response: %v", err.Error())
		}
	} else {
		Logger.Println("Zero-length response body")
	}

	Logger.Println("DecodeResponse: completed")
	return nil
}

// createHTTPClient generates the http.Client object
// from default parameters
func (r *Request) createHTTPClient() {
	Logger.Println("createHTTPClient: started")

	// Create transport for the request
	Logger.Println("Creating http.Transport")
	dial := timeoutDialer(r.Timeout)
	transport := http.Transport{
		Dial: dial,
	}

	// Create Client
	Logger.Println("Creating http.Client")
	r.Client = http.Client{
		Transport: &transport,
	}
	Logger.Println("createHTTPClient: completed")
}

// createHTTPRequest generates the actual http.Request object
// from default parameters
func (r *Request) createHTTPRequest() error {
	Logger.Println("createHTTPRequest: started")
	// Create the new request
	var err error
	r.Request, err = http.NewRequest(r.Method, r.Url, r.RequestReader)
	if err != nil {
		Logger.Println("Failed to create request:", err)
		return err
	}

	Logger.Println("createHTTPRequest: completed")
	return nil
}

// Get is a shorthand MakeRequest with method = "GET"
func Get(url string, auth Auth, ret interface{}) error {
	r := NewRequest("GET", url, auth)
	r.ResponseBody = ret
	//r.Request.Header.Set("Accept", "application/json")
	return r.Do()
}

// Post is a shorthand MakeRequest with method "POST"
func Post(url string, auth Auth, req interface{}, ret interface{}) error {
	r := NewRequest("POST", url, auth)
	r.RequestBody = req
	r.ResponseBody = ret
	//r.Request.Header.Set("Accept", "application/json")
	return r.Do()
}

// PostForm is a shorthand MakeRequest with method "POST" with form encoding
func PostForm(url string, auth Auth, req interface{}, ret interface{}) error {
	r := NewRequest("POST", url, auth)
	r.RequestBody = req
	r.ResponseBody = ret
	r.RequestType = "form"
	//r.Request.Header.Set("Accept", "application/json")
	return r.Do()
}

// Put is a shorthand MakeRequest with method "PUT"
func Put(url string, auth Auth, req interface{}, ret interface{}) error {
	r := NewRequest("PUT", url, auth)
	r.RequestBody = req
	r.ResponseBody = ret
	//r.Request.Header.Set("Accept", "application/json")
	return r.Do()
}

// Delete is a shorthand MakeRequest with method "DELETE"
func Delete(url string, auth Auth, req interface{}, ret interface{}) error {
	r := NewRequest("DELETE", url, auth)
	r.RequestBody = req
	r.ResponseBody = ret
	//r.Request.Header.Set("Accept", "application/json")
	return r.Do()
}

// Patch is a shorthand MakeRequest with method "PATCH"
func Patch(url string, auth Auth, req interface{}, ret interface{}) error {
	r := NewRequest("PATCH", url, auth)
	r.RequestBody = req
	r.ResponseBody = ret
	//r.Request.Header.Set("Accept", "application/json")
	return r.Do()
}

// timeoutDialer is a wrapper function which returns a customized
// Dial function with a built-in timer for the provided timeout
// duration
func timeoutDialer(timeout time.Duration) func(network, addr string) (net.Conn, error) {
	return func(network string, address string) (net.Conn, error) {
		return net.DialTimeout(network, address, timeout)
	}
}

// NotFoundError indicates a 404 status code was received
// from the server
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
