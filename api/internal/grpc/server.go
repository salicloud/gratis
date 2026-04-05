package grpc

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"

	agentv1 "github.com/salicloud/gratis/gen/agent/v1"
	googlegrpc "google.golang.org/grpc"
)

// Server runs the gRPC server (for agents) and HTTP API (for the frontend).
type Server struct {
	agentv1.UnimplementedAgentServiceServer

	grpcAddr string
	httpAddr string

	mu      sync.RWMutex
	agents  map[string]*connectedAgent // server_id → agent
}

type connectedAgent struct {
	serverID string
	hostname string
	stream   agentv1.AgentService_ConnectServer
	send     chan *agentv1.ServerMessage
}

func NewServer(grpcAddr, httpAddr string) *Server {
	return &Server{
		grpcAddr: grpcAddr,
		httpAddr: httpAddr,
		agents:   make(map[string]*connectedAgent),
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

// Connect implements AgentServiceServer — the main agent session handler.
func (s *Server) Connect(stream agentv1.AgentService_ConnectServer) error {
	// First message must be RegisterRequest
	msg, err := stream.Recv()
	if err != nil {
		return err
	}

	reg, ok := msg.Payload.(*agentv1.AgentMessage_Register)
	if !ok {
		return fmt.Errorf("expected RegisterRequest as first message")
	}

	serverID, err := s.authenticate(reg.Register)
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

	for {
		select {
		case <-ctx.Done():
			return nil
		case outbound := <-agent.send:
			if err := stream.Send(outbound); err != nil {
				return err
			}
		default:
			msg, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			s.handleAgentMessage(agent, msg)
		}
	}
}

func (s *Server) handleAgentMessage(agent *connectedAgent, msg *agentv1.AgentMessage) {
	switch p := msg.Payload.(type) {
	case *agentv1.AgentMessage_Heartbeat:
		slog.Debug("heartbeat",
			"server_id", agent.serverID,
			"cpu", p.Heartbeat.Metrics.GetCpuPercent(),
		)
	case *agentv1.AgentMessage_CommandResult:
		slog.Info("command result",
			"server_id", agent.serverID,
			"command_id", p.CommandResult.CommandId,
			"success", p.CommandResult.Success,
		)
	case *agentv1.AgentMessage_Log:
		slog.Info("agent log",
			"server_id", agent.serverID,
			"command_id", p.Log.CommandId,
			"msg", p.Log.Message,
		)
	}
}

func (s *Server) authenticate(req *agentv1.RegisterRequest) (serverID string, err error) {
	// TODO: validate token against database, return assigned server UUID
	if req.Token == "" {
		return "", fmt.Errorf("missing token")
	}
	// Stub: accept any non-empty token, return hostname as ID for now
	return "server-" + req.Hostname, nil
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
