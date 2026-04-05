package grpc

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	agentv1 "github.com/salicloud/gratis/gen/agent/v1"
	"github.com/salicloud/gratis/api/internal/store"
	googlegrpc "google.golang.org/grpc"
)

const commandTimeout = 30 * time.Second

// Server runs the gRPC server (for agents) and HTTP API (for the frontend).
type Server struct {
	agentv1.UnimplementedAgentServiceServer

	grpcAddr string
	httpAddr string
	adminKey string
	store    *store.Store

	mu      sync.RWMutex
	agents  map[string]*connectedAgent
	pending map[string]chan *agentv1.CommandResult
}

type connectedAgent struct {
	serverID    string
	hostname    string
	stream      agentv1.AgentService_ConnectServer
	send        chan *agentv1.ServerMessage
	lastMetrics *agentv1.SystemMetrics
	lastSeen    time.Time
}

func NewServer(grpcAddr, httpAddr, adminKey string, st *store.Store) *Server {
	return &Server{
		grpcAddr: grpcAddr,
		httpAddr: httpAddr,
		adminKey: adminKey,
		store:    st,
		agents:   make(map[string]*connectedAgent),
		pending:  make(map[string]chan *agentv1.CommandResult),
	}
}

func (s *Server) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.grpcAddr, err)
	}

	grpcSrv := googlegrpc.NewServer()
	agentv1.RegisterAgentServiceServer(grpcSrv, s)

	httpSrv := &http.Server{
		Addr:    s.httpAddr,
		Handler: s.routes(),
	}

	errCh := make(chan error, 2)

	go func() {
		slog.Info("gRPC server listening", "addr", s.grpcAddr)
		errCh <- grpcSrv.Serve(lis)
	}()

	go func() {
		slog.Info("HTTP server listening", "addr", s.httpAddr)
		if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down")
		grpcSrv.GracefulStop()
		_ = httpSrv.Shutdown(context.Background())
		return nil
	case err := <-errCh:
		return err
	}
}

// Connect implements AgentServiceServer.
func (s *Server) Connect(stream agentv1.AgentService_ConnectServer) error {
	msg, err := stream.Recv()
	if err != nil {
		return err
	}

	reg, ok := msg.Payload.(*agentv1.AgentMessage_Register)
	if !ok {
		return fmt.Errorf("expected RegisterRequest as first message")
	}

	serverID, err := s.authenticate(stream.Context(), reg.Register)
	if err != nil {
		_ = stream.Send(&agentv1.ServerMessage{
			Payload: &agentv1.ServerMessage_RegisterResponse{
				RegisterResponse: &agentv1.RegisterResponse{
					Accepted: false,
					Message:  err.Error(),
				},
			},
		})
		return err
	}

	if err := stream.Send(&agentv1.ServerMessage{
		Payload: &agentv1.ServerMessage_RegisterResponse{
			RegisterResponse: &agentv1.RegisterResponse{
				Accepted: true,
				ServerId: serverID,
			},
		},
	}); err != nil {
		return err
	}

	agent := &connectedAgent{
		serverID: serverID,
		hostname: reg.Register.Hostname,
		stream:   stream,
		send:     make(chan *agentv1.ServerMessage, 32),
	}

	s.registerAgent(agent)
	defer s.unregisterAgent(serverID)

	slog.Info("agent connected", "server_id", serverID, "hostname", agent.hostname)

	return s.handleSession(stream, agent)
}

func (s *Server) handleSession(stream agentv1.AgentService_ConnectServer, agent *connectedAgent) error {
	ctx := stream.Context()

	recv := make(chan *agentv1.AgentMessage, 8)
	recvErr := make(chan error, 1)

	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				recvErr <- err
				return
			}
			recv <- msg
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-recvErr:
			if err == io.EOF {
				return nil
			}
			return err
		case outbound := <-agent.send:
			if err := stream.Send(outbound); err != nil {
				return err
			}
		case msg := <-recv:
			s.handleAgentMessage(agent, msg)
		}
	}
}

func (s *Server) handleAgentMessage(agent *connectedAgent, msg *agentv1.AgentMessage) {
	switch p := msg.Payload.(type) {
	case *agentv1.AgentMessage_Heartbeat:
		s.mu.Lock()
		agent.lastMetrics = p.Heartbeat.Metrics
		agent.lastSeen = time.Now()
		s.mu.Unlock()
		slog.Debug("heartbeat",
			"server_id", agent.serverID,
			"cpu", p.Heartbeat.Metrics.GetCpuPercent(),
		)
	case *agentv1.AgentMessage_CommandResult:
		result := p.CommandResult
		slog.Info("command result",
			"server_id", agent.serverID,
			"command_id", result.CommandId,
			"success", result.Success,
		)
		s.mu.Lock()
		ch, ok := s.pending[result.CommandId]
		if ok {
			delete(s.pending, result.CommandId)
		}
		s.mu.Unlock()
		if ok {
			ch <- result
		}
	case *agentv1.AgentMessage_Log:
		slog.Info("agent log",
			"server_id", agent.serverID,
			"command_id", p.Log.CommandId,
			"msg", p.Log.Message,
		)
	}
}

// SendCommand dispatches a command to the named server and waits for the result.
func (s *Server) SendCommand(ctx context.Context, serverID string, cmd *agentv1.Command) (*agentv1.CommandResult, error) {
	s.mu.RLock()
	agent, ok := s.agents[serverID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %q not connected", serverID)
	}

	cmd.CommandId = uuid.NewString()

	resultCh := make(chan *agentv1.CommandResult, 1)
	s.mu.Lock()
	s.pending[cmd.CommandId] = resultCh
	s.mu.Unlock()

	agent.send <- &agentv1.ServerMessage{
		Payload: &agentv1.ServerMessage_Command{Command: cmd},
	}

	cmdCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	select {
	case result := <-resultCh:
		return result, nil
	case <-cmdCtx.Done():
		s.mu.Lock()
		delete(s.pending, cmd.CommandId)
		s.mu.Unlock()
		return nil, fmt.Errorf("command timed out after %s", commandTimeout)
	}
}

func (s *Server) authenticate(ctx context.Context, req *agentv1.RegisterRequest) (serverID string, err error) {
	if req.Token == "" {
		return "", fmt.Errorf("missing token")
	}
	if s.store == nil {
		// No store configured — accept any non-empty token (dev mode)
		return "server-" + req.Hostname, nil
	}
	serverID, err = s.store.ValidateToken(ctx, req.Token)
	if err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}
	if err := s.store.UpsertServer(ctx, serverID, req.Hostname); err != nil {
		slog.Warn("failed to upsert server record", "err", err)
	}
	return serverID, nil
}

func (s *Server) registerAgent(agent *connectedAgent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[agent.serverID] = agent
}

func (s *Server) unregisterAgent(serverID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agents, serverID)
	slog.Info("agent disconnected", "server_id", serverID)
}
