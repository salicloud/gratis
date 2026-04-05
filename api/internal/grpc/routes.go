package grpc

import (
	"encoding/json"
	"net/http"
	"time"

	agentv1 "github.com/salicloud/gratis/gen/agent/v1"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /api/v1/admin/tokens", s.handleCreateToken)
	mux.HandleFunc("GET /api/v1/servers", s.handleListServers)
	mux.HandleFunc("GET /api/v1/servers/{server_id}", s.handleGetServer)
	mux.HandleFunc("POST /api/v1/servers/{server_id}/vhosts", s.handleCreateVhost)
	mux.HandleFunc("DELETE /api/v1/servers/{server_id}/vhosts/{domain}", s.handleDeleteVhost)

	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleCreateToken creates a provisioning token for a new server.
// Protected by the GRATIS_ADMIN_KEY header.
func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	if s.adminKey == "" || r.Header.Get("X-Admin-Key") != s.adminKey {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.store == nil {
		http.Error(w, "store not configured", http.StatusServiceUnavailable)
		return
	}
	token, err := s.store.CreateToken(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
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

func (s *Server) handleGetServer(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("server_id")

	s.mu.RLock()
	agent, ok := s.agents[serverID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	type metrics struct {
		CPUPercent float64 `json:"cpu_percent"`
		MemTotal   uint64  `json:"mem_total"`
		MemUsed    uint64  `json:"mem_used"`
		DiskTotal  uint64  `json:"disk_total"`
		DiskUsed   uint64  `json:"disk_used"`
		Load1      float64 `json:"load_1"`
		Load5      float64 `json:"load_5"`
		Load15     float64 `json:"load_15"`
	}
	type response struct {
		ServerID string    `json:"server_id"`
		Hostname string    `json:"hostname"`
		Online   bool      `json:"online"`
		LastSeen string    `json:"last_seen"`
		Metrics  *metrics  `json:"metrics,omitempty"`
	}

	resp := response{
		ServerID: agent.serverID,
		Hostname: agent.hostname,
		Online:   true,
		LastSeen: agent.lastSeen.UTC().Format(time.RFC3339),
	}
	if agent.lastMetrics != nil {
		m := agent.lastMetrics
		resp.Metrics = &metrics{
			CPUPercent: m.CpuPercent,
			MemTotal:   m.MemTotal,
			MemUsed:    m.MemUsed,
			DiskTotal:  m.DiskTotal,
			DiskUsed:   m.DiskUsed,
			Load1:      m.Load_1,
			Load5:      m.Load_5,
			Load15:     m.Load_15,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCreateVhost(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("server_id")

	var body struct {
		Username   string   `json:"username"`
		Domain     string   `json:"domain"`
		Docroot    string   `json:"docroot"`
		PHPVersion string   `json:"php_version"`
		Aliases    []string `json:"aliases"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Domain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}

	result, err := s.SendCommand(r.Context(), serverID, &agentv1.Command{
		Payload: &agentv1.Command_CreateVhost{
			CreateVhost: &agentv1.CreateVhostCmd{
				Username:   body.Username,
				Domain:     body.Domain,
				Docroot:    body.Docroot,
				PhpVersion: body.PHPVersion,
				Aliases:    body.Aliases,
			},
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !result.Success {
		http.Error(w, result.Error, http.StatusUnprocessableEntity)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleDeleteVhost(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("server_id")
	domain := r.PathValue("domain")

	result, err := s.SendCommand(r.Context(), serverID, &agentv1.Command{
		Payload: &agentv1.Command_DeleteVhost{
			DeleteVhost: &agentv1.DeleteVhostCmd{Domain: domain},
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !result.Success {
		http.Error(w, result.Error, http.StatusUnprocessableEntity)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
