package calendar

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/grandcat/zeroconf"
)

const (
	AppServiceType   = "_localitas-app._tcp"
	AppServiceDomain = "local."
)

type AppHealth struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Icon        string `json:"icon"`
	Version     string `json:"version"`
	Status      string `json:"status"`
}

var DefaultHealth = AppHealth{
	Name:        "calendar",
	DisplayName: "Calendar",
	Icon:        "calendar",
	Version:     "0.1.0",
	Status:      "healthy",
}

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(DefaultHealth)
}

func BroadcastMDNS(port int, name string) (shutdown func(), err error) {
	txt := []string{fmt.Sprintf("name=%s", name)}
	server, err := zeroconf.Register(name, AppServiceType, AppServiceDomain, port, txt, nil)
	if err != nil {
		return nil, fmt.Errorf("mDNS register: %w", err)
	}
	logger.Info("broadcasting mDNS", "service_type", AppServiceType, "port", port, "name", name)
	return server.Shutdown, nil
}
