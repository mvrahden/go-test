package assert

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
)

func httpOK(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func httpReadBody(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("hello"))
}

func httpRedirect(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func httpError(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
}

func httpStatusCode(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusSwitchingProtocols)
}

func TestHTTPSuccess(t *testing.T) {
	t.Parallel()

	mockT1 := new(testing.T)
	Equal(t, HTTPSuccess(mockT1, httpOK, "GET", "/", nil), true)
	False(t, mockT1.Failed())

	mockT2 := new(testing.T)
	Equal(t, HTTPSuccess(mockT2, httpRedirect, "GET", "/", nil), false)
	True(t, mockT2.Failed())

	mockT3 := new(mockTestingT)
	Equal(t, HTTPSuccess(
		mockT3, httpError, "GET", "/", nil,
		"was not expecting a failure here",
	), false)
	True(t, mockT3.Failed())
	Contains(t, mockT3.errorString(), "was not expecting a failure here")

	mockT4 := new(testing.T)
	Equal(t, HTTPSuccess(mockT4, httpStatusCode, "GET", "/", nil), false)
	True(t, mockT4.Failed())

	mockT5 := new(testing.T)
	Equal(t, HTTPSuccess(mockT5, httpReadBody, "POST", "/", nil), true)
	False(t, mockT5.Failed())
}

func TestHTTPRedirect(t *testing.T) {
	t.Parallel()

	mockT1 := new(mockTestingT)
	Equal(t, HTTPRedirect(
		mockT1, httpOK, "GET", "/", nil,
		"was expecting a 3xx status code. Got 200.",
	), false)
	True(t, mockT1.Failed())
	Contains(t, mockT1.errorString(), "was expecting a 3xx status code. Got 200.")

	mockT2 := new(testing.T)
	Equal(t, HTTPRedirect(mockT2, httpRedirect, "GET", "/", nil), true)
	False(t, mockT2.Failed())

	mockT3 := new(testing.T)
	Equal(t, HTTPRedirect(mockT3, httpError, "GET", "/", nil), false)
	True(t, mockT3.Failed())

	mockT4 := new(testing.T)
	Equal(t, HTTPRedirect(mockT4, httpStatusCode, "GET", "/", nil), false)
	True(t, mockT4.Failed())
}

func TestHTTPError(t *testing.T) {
	t.Parallel()

	mockT1 := new(testing.T)
	Equal(t, HTTPError(mockT1, httpOK, "GET", "/", nil), false)
	True(t, mockT1.Failed())

	mockT2 := new(mockTestingT)
	Equal(t, HTTPError(
		mockT2, httpRedirect, "GET", "/", nil,
		"Expected this request to error out. But it didn't",
	), false)
	True(t, mockT2.Failed())
	Contains(t, mockT2.errorString(), "Expected this request to error out. But it didn't")

	mockT3 := new(testing.T)
	Equal(t, HTTPError(mockT3, httpError, "GET", "/", nil), true)
	False(t, mockT3.Failed())

	mockT4 := new(testing.T)
	Equal(t, HTTPError(mockT4, httpStatusCode, "GET", "/", nil), false)
	True(t, mockT4.Failed())
}

func TestHTTPStatusCode(t *testing.T) {
	t.Parallel()

	mockT1 := new(testing.T)
	Equal(t, HTTPStatusCode(mockT1, httpOK, "GET", "/", nil, http.StatusSwitchingProtocols), false)
	True(t, mockT1.Failed())

	mockT2 := new(testing.T)
	Equal(t, HTTPStatusCode(mockT2, httpRedirect, "GET", "/", nil, http.StatusSwitchingProtocols), false)
	True(t, mockT2.Failed())

	mockT3 := new(mockTestingT)
	Equal(t, HTTPStatusCode(
		mockT3, httpError, "GET", "/", nil, http.StatusSwitchingProtocols,
		"Expected the status code to be %d", http.StatusSwitchingProtocols,
	), false)
	True(t, mockT3.Failed())
	Contains(t, mockT3.errorString(), "Expected the status code to be 101")

	mockT4 := new(testing.T)
	Equal(t, HTTPStatusCode(mockT4, httpStatusCode, "GET", "/", nil, http.StatusSwitchingProtocols), true)
	False(t, mockT4.Failed())
}

func TestHTTPStatusesWrapper(t *testing.T) {
	t.Parallel()

	mockT := new(testing.T)

	Equal(t, HTTPSuccess(mockT, httpOK, "GET", "/", nil), true)
	Equal(t, HTTPSuccess(mockT, httpRedirect, "GET", "/", nil), false)
	Equal(t, HTTPSuccess(mockT, httpError, "GET", "/", nil), false)

	Equal(t, HTTPRedirect(mockT, httpOK, "GET", "/", nil), false)
	Equal(t, HTTPRedirect(mockT, httpRedirect, "GET", "/", nil), true)
	Equal(t, HTTPRedirect(mockT, httpError, "GET", "/", nil), false)

	Equal(t, HTTPError(mockT, httpOK, "GET", "/", nil), false)
	Equal(t, HTTPError(mockT, httpRedirect, "GET", "/", nil), false)
	Equal(t, HTTPError(mockT, httpError, "GET", "/", nil), true)
}

func httpHelloName(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	_, _ = fmt.Fprintf(w, "Hello, %s!", name)
}

func TestHTTPRequestWithNoParams(t *testing.T) {
	t.Parallel()

	var got *http.Request
	handler := func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(http.StatusOK)
	}

	True(t, HTTPSuccess(t, handler, "GET", "/url", nil))

	Empty(t, got.URL.Query())
	Equal(t, "/url", got.URL.RequestURI())
}

func TestHTTPRequestWithParams(t *testing.T) {
	t.Parallel()

	var got *http.Request
	handler := func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(http.StatusOK)
	}
	params := url.Values{}
	params.Add("id", "12345")

	True(t, HTTPSuccess(t, handler, "GET", "/url", params))

	Equal(t, url.Values{"id": []string{"12345"}}, got.URL.Query())
	Equal(t, "/url?id=12345", got.URL.String())
	Equal(t, "/url?id=12345", got.URL.RequestURI())
}

func TestHttpBody(t *testing.T) {
	t.Parallel()

	mockT := new(mockTestingT)

	True(t, HTTPBodyContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "Hello, World!"))
	True(t, HTTPBodyContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "World"))
	False(t, HTTPBodyContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "world"))

	False(t, HTTPBodyNotContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "Hello, World!"))
	False(t, HTTPBodyNotContains(
		mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "World",
		"Expected the request body to not contain 'World'. But it did.",
	))
	True(t, HTTPBodyNotContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "world"))
	Contains(t, mockT.errorString(), "Expected the request body to not contain 'World'. But it did.")

	True(t, HTTPBodyContains(mockT, httpReadBody, "GET", "/", nil, "hello"))
}

func TestHttpBodyWrappers(t *testing.T) {
	t.Parallel()

	mockT := new(testing.T)

	True(t, HTTPBodyContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "Hello, World!"))
	True(t, HTTPBodyContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "World"))
	False(t, HTTPBodyContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "world"))

	False(t, HTTPBodyNotContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "Hello, World!"))
	False(t, HTTPBodyNotContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "World"))
	True(t, HTTPBodyNotContains(mockT, httpHelloName, "GET", "/", url.Values{"name": []string{"World"}}, "world"))
}
