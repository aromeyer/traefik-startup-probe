package main

import (
	"bytes"
	"io"
	"net/http"
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
	client := &MockClient{
		MockDo: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte(json))),
			}, nil
		},
	}

	r := NewRouterCounter()
	err := r.countRoutersPerProvider(client)
	if err != nil {
		t.Error(err)
		return
	}

	assert.Equal(t, 3, r.CountPerProvider["internal"])
	assert.Equal(t, 0, r.PreviousCountPerProvider["internal"])

	assert.Equal(t, 1, r.CountPerProvider["docker"])
	assert.Equal(t, 0, r.PreviousCountPerProvider["docker"])

	assert.False(t, r.ServerInitialized)

	r.UpdateServerState()
	assert.False(t, r.ServerInitialized)

	err = r.countRoutersPerProvider(client)
	if err != nil {
		t.Error(err)
		return
	}

	assert.Equal(t, 3, r.CountPerProvider["internal"])
	assert.Equal(t, 3, r.PreviousCountPerProvider["internal"])

	assert.Equal(t, 1, r.CountPerProvider["docker"])
	assert.Equal(t, 1, r.PreviousCountPerProvider["docker"])

	assert.False(t, r.ServerInitialized)

	r.UpdateServerState()
	assert.True(t, r.ServerInitialized)
}
