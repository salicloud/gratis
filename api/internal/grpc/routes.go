package grpc

import (
	"encoding/json"
	"net/http"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/v1/servers", s.handleListServers)

	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleListServers(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type serverInfo struct {
		ServerID string `json:"server_id"`
		Hostname string `json:"hostname"`
	}

	servers := make([]serverInfo, 0, len(s.agents))
	for _, a := range s.agents {
		servers = append(servers, serverInfo{
			ServerID: a.serverID,
			Hostname: a.hostname,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(servers)
}
