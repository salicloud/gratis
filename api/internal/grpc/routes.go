package grpc

import (
	"encoding/json"
	"net/http"

	agentv1 "github.com/salicloud/gratis/gen/agent/v1"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/v1/servers", s.handleListServers)
	mux.HandleFunc("POST /api/v1/servers/{server_id}/vhosts", s.handleCreateVhost)
	mux.HandleFunc("DELETE /api/v1/servers/{server_id}/vhosts/{domain}", s.handleDeleteVhost)

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
