package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

const (
	INTERNAL_PROVIDER_NAME = "internal"
)

type RawData struct {
	Routers map[string]Router `json:"routers"`
}

type Router struct {
}

type RouterCounter struct {
	CountPerProvider         map[string]int
	PreviousCountPerProvider map[string]int
	ServerInitialized        bool
	mu                       sync.RWMutex
}

// NewRouterCounter initializes a new RouterCounter instance.
func NewRouterCounter() *RouterCounter {
	return &RouterCounter{
		CountPerProvider:         map[string]int{},
		PreviousCountPerProvider: map[string]int{},
		ServerInitialized:        false,
	}
}

// HTTPClient interface abstracts the HTTP client for testing purposes.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// countRoutersPerProvider updates the router counts per provider.
func (r *RouterCounter) countRoutersPerProvider(client HTTPClient) error {
	var rawData RawData

	req, err := http.NewRequest(http.MethodGet, "http://localhost:8080/api/rawdata", nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&rawData); err != nil {
		return fmt.Errorf("failed to decode response body: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Backup previous counts
	r.PreviousCountPerProvider = map[string]int{}
	for providerName, count := range r.CountPerProvider {
		r.PreviousCountPerProvider[providerName] = count
	}

	// Reset and update current counts
	r.CountPerProvider = map[string]int{}
	for name := range rawData.Routers {
		if lastIndex := strings.LastIndex(name, "@"); lastIndex > 0 {
			providerName := name[lastIndex+1:]
			r.CountPerProvider[providerName]++
		}
	}

	return nil
}

// IsServerInitialized checks if the server is fully initialized.
func (r *RouterCounter) UpdateServerState() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	countProviderSuccess := 0
	for name, count := range r.CountPerProvider {
		if count == r.PreviousCountPerProvider[name] {
			countProviderSuccess++
		}
	}

	// All providers are loaded and internal provider is not empty
	if countProviderSuccess == len(r.CountPerProvider) && r.CountPerProvider[INTERNAL_PROVIDER_NAME] > 0 {
		r.ServerInitialized = true
	} else {
		r.ServerInitialized = false
	}
}

func main() {
	// start with Service Unavailable
	code := http.StatusServiceUnavailable

	routerCounter := NewRouterCounter()

	logLevelString, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		logLevelString = "info"
	}

	logLevel, err := log.ParseLevel(logLevelString)
	if err != nil {
		logLevel = log.DebugLevel
	}
	log.SetLevel(logLevel)

	log.Info("startup probe server started")

	client := &http.Client{}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !routerCounter.ServerInitialized {
			if err = routerCounter.countRoutersPerProvider(client); err == nil {
				log.Debug(fmt.Sprintf("routerCounter: %v (previous: %v)", routerCounter.CountPerProvider, routerCounter.PreviousCountPerProvider))

				routerCounter.UpdateServerState()
				if routerCounter.ServerInitialized {
					code = http.StatusOK
				}
			} else {
				log.Error(err)
			}
		}

		log.Debug(fmt.Sprintf("serverInitialized: %v", routerCounter.ServerInitialized))

		w.WriteHeader(code)
		_, err = w.Write([]byte(http.StatusText(code)))
		if err != nil {
			log.Error(err)
		}

		log.Info(fmt.Sprintf("%v %v %v", r.URL, code, r.UserAgent()))

	})

	log.Fatal(http.ListenAndServe(":8083", nil))
}
