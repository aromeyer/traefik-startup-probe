package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	LOG_LEVEL                  = getEnvWithDefault("LOG_LEVEL", "info")
	TRAEFIK_RAWDATA_ENDPOINT   = getEnvWithDefault("TRAEFIK_RAWDATA_ENDPOINT", "http://localhost:8080/api/rawdata")
	HTTP_SERVER_ADDR           = getEnvWithDefault("HTTP_SERVER_ADDR", ":8083")
	TRAEFIK_PROVIDER_NOT_EMPTY = getEnvStrSliceWithDefault("TRAEFIK_PROVIDER_NOT_EMPTY", []string{"internal"})
)

type RawData struct {
	Routers map[string]Router `json:"routers"`
}

type Router struct {
}

//
// RouterCounter
//

type RouterCounter struct {
	CurrentCountPerProvider  map[string]int
	PreviousCountPerProvider map[string]int
	mu                       sync.RWMutex
	ServerInitialized        bool
	NCalls                   int
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// NewRouterCounter initializes a new RouterCounter instance.
func NewRouterCounter() *RouterCounter {
	return &RouterCounter{
		CurrentCountPerProvider:  map[string]int{},
		PreviousCountPerProvider: map[string]int{},
		ServerInitialized:        false,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
}

// countRoutersPerProvider updates the router counts per provider.
func (r *RouterCounter) countRoutersPerProvider(client HTTPClient) error {
	var rawData RawData

	r.NCalls++

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
	for providerName, count := range r.CurrentCountPerProvider {
		r.PreviousCountPerProvider[providerName] = count
	}

	// Reset and update current counts
	r.CurrentCountPerProvider = map[string]int{}
	for name := range rawData.Routers {
		if lastIndex := strings.LastIndex(name, "@"); lastIndex > 0 {
			providerName := name[lastIndex+1:]
			r.CurrentCountPerProvider[providerName]++
		}
	}

	r.UpdatedAt = time.Now()

	return nil
}

// checks if the server is fully initialized.
func (r *RouterCounter) UpdateServerStatus() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	countProviderSuccess := 0
	for name, count := range r.CurrentCountPerProvider {
		if count == r.PreviousCountPerProvider[name] {
			countProviderSuccess++
		}
	}

	// All providers are loaded and internal provider is not empty
	if countProviderSuccess == len(r.CurrentCountPerProvider) {
		foundEmpty := false
		for _, p := range TRAEFIK_PROVIDER_NOT_EMPTY {
			if v, ok := r.CurrentCountPerProvider[p]; !ok || v == 0 {
				foundEmpty = true
				log.Debugf("provider with name %s not found or empty - CurrentCountPerProvider: %v", p, r.CurrentCountPerProvider)
				break
			}
		}
		if !foundEmpty {
			r.ServerInitialized = true
		}
	} else {
		log.Debugf("provider status list not yet stable (%d != %d)", countProviderSuccess, len(r.CurrentCountPerProvider))
		r.ServerInitialized = false
	}
}

//
// Utils
//

// set variable value from env var with default
func getEnvWithDefault(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return defaultValue
}

func getEnvStrSliceWithDefault(key string, defaultValue []string) []string {
	if v, ok := os.LookupEnv(key); ok {
		return strings.Split(v, ",")
	}

	return defaultValue
}

// HTTPClient interface abstracts the HTTP client for testing purposes.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

//
// main
//

func main() {
	PrintConfig()

	routerCounter := NewRouterCounter()

	logLevel, err := log.ParseLevel(LOG_LEVEL)
	if err != nil {
		logLevel = log.DebugLevel
	}
	log.SetLevel(logLevel)

	log.Info("startup probe server started")

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	http.HandleFunc("/healthz", createHealthzHandler(routerCounter, client))

	log.Fatal(http.ListenAndServe(HTTP_SERVER_ADDR, nil))
}

func createHealthzHandler(routerCounter *RouterCounter, client HTTPClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := http.StatusServiceUnavailable

		if !routerCounter.ServerInitialized {
			if err := routerCounter.countRoutersPerProvider(client); err == nil {
				routerCounter.UpdateServerStatus()
				if routerCounter.ServerInitialized {
					code = http.StatusOK
				}
			} else {
				log.Error(err)
			}
		} else { // once successfully initialized the status doesn't change
			code = http.StatusOK
		}

		log.Debugf(
			"routerCounter - ServerInitialized: %t - nCalls: %d - current: %v (previous: %v) - elapsed: %s - initDuration: %s",
			routerCounter.ServerInitialized,
			routerCounter.NCalls,
			routerCounter.CurrentCountPerProvider,
			routerCounter.PreviousCountPerProvider,
			time.Since(routerCounter.CreatedAt),
			routerCounter.UpdatedAt.Sub(routerCounter.CreatedAt),
		)

		w.WriteHeader(code)
		_, err := w.Write([]byte(http.StatusText(code)))
		if err != nil {
			log.Error(err)
		}

		log.Infof("%v %v %v", r.URL, code, r.UserAgent())
	}
}

func PrintConfig() {
	log.Info("-- CONFIG")
	log.Infof("LOG_LEVEL: %s", LOG_LEVEL)
	log.Infof("HTTP_SERVER_ADDR: %s", HTTP_SERVER_ADDR)
	log.Infof("TRAEFIK_RAWDATA_ENDPOINT: %s", TRAEFIK_RAWDATA_ENDPOINT)
	log.Infof("TRAEFIK_PROVIDER_NOT_EMPTY: %s", TRAEFIK_PROVIDER_NOT_EMPTY)
	log.Info("-- ")
}
