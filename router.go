package main

import (
	"os"
	"regexp"
	"sync"

	"github.com/goccy/go-json"
)

type RouteConfig struct {
	Pattern  string `json:"pattern"`            // Regex pattern for model name
	Upstream string `json:"upstream"`           // Base URL
	AuthKey  string `json:"auth_key,omitempty"` // Optional override auth key for this upstream
}

var (
	modelRoutes []RouteConfig
	routesMutex sync.RWMutex
)

func loadModelRoutes() error {
	data, err := os.ReadFile("routes.json")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var routes []RouteConfig
	if err := json.Unmarshal(data, &routes); err != nil {
		return err
	}

	routesMutex.Lock()
	modelRoutes = routes
	routesMutex.Unlock()
	return nil
}

func getUpstreamForModel(model string, defaultBase string) (string, string) {
	routesMutex.RLock()
	defer routesMutex.RUnlock()

	for _, route := range modelRoutes {
		if matched, _ := regexp.MatchString(route.Pattern, model); matched {
			return route.Upstream, route.AuthKey
		}
	}
	return defaultBase, ""
}
