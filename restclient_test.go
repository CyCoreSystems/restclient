package restclient

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

var auth = new(Auth)

func init() {
	auth.Username = "edward"
	auth.Password = "pass"
}

func TestNewRequest(t *testing.T) {
	assert := assert.New(t)
	req := NewRequest("GET", "url.com", *auth)
	AuthTester(t, *auth, req.Auth)
	assert.Equal(req.Method, "GET", "Method should match call")
	assert.Equal(req.Url, "url.com", "URL should match call")
}

//this is simply a guarantee that no authentication is ever mangled.
func AuthTester(t *testing.T, auth Auth, reqAuth Auth) {
	assert := assert.New(t)
	assert.Equal(auth.Username, reqAuth.Username, "Usernames should be equal")
	assert.Equal(auth.Password, reqAuth.Password, "Passwords should be equal")
}

/*
	Some simple tests to guarantee no data is somehow mangled on transport
*/

func TestCreateRequest(t *testing.T) {
	assert := assert.New(t)
	req := NewRequest("GET", "url.com", *auth)
	err := req.createHTTPRequest()
	assert.Nil(err)
	assert.Equal(req.Request.Method, "GET", "Method should match call")
	assert.Equal(req.Request.URL.Path, "url.com", "URL should match call")
	assert.Equal(req.Request.Proto, "HTTP/1.1")
	assert.NotNil(req.Request.Header)
}

func TestCreateClient(t *testing.T) {
	assert := assert.New(t)
	req := NewRequest("GET", "url.com", *auth)
	req.createHTTPClient()
	assert.NotNil(req.Client)
	assert.NotNil(req.Client.Transport)
}

type TestStructRequest struct {
	Variable string `json:"variable"`
}

/*
	Create an arbitrary json-ified struct and pass it through.
	The encoder should not register any errors and should return
	a map (non-nil), on success.
*/

func TestEncodeRequest(t *testing.T) {
	assert := assert.New(t)
	req := NewRequest("GET", "url.com", *auth)
	req.RequestBody = TestStructRequest{"hi"}
	err := req.EncodeRequestBody()
	assert.Nil(err)
	assert.NotNil(req.RequestReader)
}

/*
	This method just tries a few different possible status codes.
	All we're testing for now is that none of the errors on response
	are nil, unless we pass through a 200.
*/
func TestProcessStatusCode(t *testing.T) {
	assert := assert.New(t)
	req := NewRequest("GET", "url.com", *auth)
	req.Response = new(http.Response)
	req.Response.StatusCode = 404
	err := req.ProcessStatusCode()
	assert.NotNil(err)
	req.Response.StatusCode = 450
	err = req.ProcessStatusCode()
	assert.NotNil(err)
	req.Response.StatusCode = 550
	err = req.ProcessStatusCode()
	assert.NotNil(err)
	req.Response.StatusCode = 200
	err = req.ProcessStatusCode()
	assert.Nil(err)
}

/*
 */

func TestDecodeResponse(t *testing.T) {
	assert := assert.New(t)
	req := NewRequest("GET", "url.com", *auth)
	assert.NotNil(req)
}
