package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type MockDoType func(req *http.Request) (*http.Response, error)

type MockClient struct {
	MockDo MockDoType
}

func (m *MockClient) Do(req *http.Request) (*http.Response, error) {
	return m.MockDo(req)
}
func TestHealthzHandler(t *testing.T) {
	mockClient := &MockClient{
		MockDo: func(req *http.Request) (*http.Response, error) {
			json := `{
				"routers": {
					"api@internal": {},
					"backend@docker": {}
				}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(json))),
			}, nil
		},
	}

	r := NewRouterCounter()
	handler := createHealthzHandler(r, mockClient)

	// First request: server not initialized
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "Expected Service Unavailable before initialization")
	assert.Equal(t, "Service Unavailable", rec.Body.String(), "Expected Service Unavailable message before initialization")

	// Second request: server initialized
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "Expected OK after initialization")
	assert.Equal(t, "OK", rec.Body.String(), "Expected OK message after initialization")
}

func TestRouterCounter(t *testing.T) {
	json := `
{
   "routers":{
      "api@internal":{
         "entryPoints":[
            "traefik"
         ],
         "service":"api@internal"
      },
      "backend@docker":{
         "entryPoints":[
            "websecure"
         ],
         "service":"backend"
      },
      "dashboard@internal":{
         "entryPoints":[
            "traefik"
         ],
         "service":"dashboard@internal"
      },
      "prometheus@internal":{
         "entryPoints":[
            "traefik"
         ],
         "service":"prometheus@internal"
      }
   },
   "middlewares":{
      "dashboard_redirect@internal":{

      },
      "dashboard_stripprefix@internal":{

      }
   },
   "services":{
      "api@internal":{

      },
      "dashboard@internal":{

      },
      "noop@internal":{

      },
      "prometheus@internal":{

      }
   }
}
	`
	mockClient := &MockClient{
		MockDo: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(json))),
			}, nil
		},
	}

	r := NewRouterCounter()
	err := r.countRoutersPerProvider(mockClient)
	if err != nil {
		t.Error(err)
		return
	}

	assert.Equal(t, 3, r.CurrentCountPerProvider["internal"])
	assert.Equal(t, 0, r.PreviousCountPerProvider["internal"])

	assert.Equal(t, 1, r.CurrentCountPerProvider["docker"])
	assert.Equal(t, 0, r.PreviousCountPerProvider["docker"])

	assert.False(t, r.ServerInitialized)

	r.UpdateServerStatus()
	assert.False(t, r.ServerInitialized)

	err = r.countRoutersPerProvider(mockClient)
	if err != nil {
		t.Error(err)
		return
	}

	assert.Equal(t, 3, r.CurrentCountPerProvider["internal"])
	assert.Equal(t, 3, r.PreviousCountPerProvider["internal"])

	assert.Equal(t, 1, r.CurrentCountPerProvider["docker"])
	assert.Equal(t, 1, r.PreviousCountPerProvider["docker"])

	assert.False(t, r.ServerInitialized)

	r.UpdateServerStatus()
	assert.True(t, r.ServerInitialized)
}
