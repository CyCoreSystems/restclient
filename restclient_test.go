package restclient

import (
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

//this method exists solely to clean up the amount of auth tests.
//this is simply a guarantee that no authentication is ever mangled.
func AuthTester(t *testing.T, auth Auth, reqAuth Auth) {
	assert := assert.New(t)
	assert.Equal(auth.Username, reqAuth.Username, "Usernames should be equal")
	assert.Equal(auth.Password, reqAuth.Password, "Passwords should be equal")
}

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

type TestStruct struct {
	Variable string `json:"variable"`
}

func TestEncodeRequest(t *testing.T) {
	assert := assert.New(t)
	req := NewRequest("GET", "url.com", *auth)
	req.RequestBody = TestStruct{"hi"}
	err := req.EncodeRequestBody()
	assert.Nil(err)
	assert.NotNil(req.RequestReader)
}
