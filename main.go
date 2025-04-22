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

var (
	LOG_LEVEL                = getEnvWithDefault("LOG_LEVEL", "info")
	TRAEFIK_RAWDATA_ENDPOINT = getEnvWithDefault("TRAEFIK_RAWDATA_ENDPOINT", "http://localhost:8080/api/rawdata")
	HTTP_SERVER_ADDR         = getEnvWithDefault("HTTP_SERVER_ADDR", ":8083")
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

// countRoutersPerProvider updates the router counts per provider.
func (r *RouterCounter) countRoutersPerProvider(client HTTPClient) error {
	var rawData RawData

	req, err := http.NewRequest(http.MethodGet, TRAEFIK_RAWDATA_ENDPOINT, nil)
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

// set variable value from env var with default
func getEnvWithDefault(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return defaultValue
}

// HTTPClient interface abstracts the HTTP client for testing purposes.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func main() {

	routerCounter := NewRouterCounter()

	logLevel, err := log.ParseLevel(LOG_LEVEL)
	if err != nil {
		logLevel = log.DebugLevel
	}
	log.SetLevel(logLevel)

	log.Info("startup probe server started")

	client := &http.Client{}

	http.HandleFunc("/healthz", createHealthzHandler(routerCounter, client))

	log.Fatal(http.ListenAndServe(HTTP_SERVER_ADDR, nil))
}

func createHealthzHandler(routerCounter *RouterCounter, client HTTPClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := http.StatusServiceUnavailable

		if !routerCounter.ServerInitialized {
			if err := routerCounter.countRoutersPerProvider(client); err == nil {
				log.Debug(fmt.Sprintf("routerCounter: %v (previous: %v)", routerCounter.CountPerProvider, routerCounter.PreviousCountPerProvider))

				routerCounter.UpdateServerState()
				if routerCounter.ServerInitialized {
					code = http.StatusOK
				}
			} else {
				log.Error(err)
			}
		} else { // once successfully initialized the state doesn't change
			code = http.StatusOK
		}

		log.Debug(fmt.Sprintf("serverInitialized: %v", routerCounter.ServerInitialized))

		w.WriteHeader(code)
		_, err := w.Write([]byte(http.StatusText(code)))
		if err != nil {
			log.Error(err)
		}

		log.Info(fmt.Sprintf("%v %v %v", r.URL, code, r.UserAgent()))
	}
}
