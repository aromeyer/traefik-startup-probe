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

func NewRouterCounter() *RouterCounter {
	return &RouterCounter{
		CountPerProvider:         map[string]int{},
		PreviousCountPerProvider: map[string]int{},
		ServerInitialized:        false,
	}
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func (r *RouterCounter) countRoutersPerProvider(client HTTPClient) error {

	var rawData RawData

	req, err := http.NewRequest(http.MethodGet, "http://localhost:8080/api/rawdata", nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&rawData)

	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.PreviousCountPerProvider = map[string]int{}
	for providerName, count := range r.CountPerProvider {
		r.PreviousCountPerProvider[providerName] = count
	}

	r.CountPerProvider = map[string]int{}
	for name := range rawData.Routers {
		lastIndex := strings.LastIndex(name, "@")
		if lastIndex > 0 {
			providerName := name[lastIndex+1:]
			r.CountPerProvider[providerName] += 1
		}
	}

	return nil
}

func (r *RouterCounter) IsServerInitialized() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	countProviderSuccess := 0
	for name, count := range r.CountPerProvider {
		if count == r.PreviousCountPerProvider[name] {
			countProviderSuccess += 1
		}
	}

	if countProviderSuccess == len(r.CountPerProvider) {
		r.ServerInitialized = true
	}

	return r.ServerInitialized
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
			if err = routerCounter.countRoutersPerProvider(client); err != nil {
				log.Error(err)
			}
			log.Debug(fmt.Sprintf("routerCounter: %v (previous: %v)", routerCounter.CountPerProvider, routerCounter.PreviousCountPerProvider))
			routerCounter.mu.RLock()
			defer routerCounter.mu.RUnlock()

			if routerCounter.IsServerInitialized() {
				code = http.StatusOK
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
