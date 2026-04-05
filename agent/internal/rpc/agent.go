package rpc

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"time"

	agentv1 "github.com/salicloud/gratis/gen/agent/v1"
	"github.com/salicloud/gratis/agent/internal/system"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	heartbeatInterval = 15 * time.Second
	reconnectDelay    = 5 * time.Second
)

type Agent struct {
	apiAddr   string
	token     string
	lastCPU   system.CPUSample
}

func NewAgent(apiAddr, token string) *Agent {
	return &Agent{apiAddr: apiAddr, token: token}
}

// Run connects to the API and maintains the session, reconnecting on failure.
func (a *Agent) Run(ctx context.Context) error {
	for {
		if err := a.connect(ctx); err != nil {
			slog.Error("session ended", "err", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(reconnectDelay):
			slog.Info("reconnecting", "addr", a.apiAddr)
		}
	}
}

func (a *Agent) connect(ctx context.Context) error {
	conn, err := grpc.NewClient(a.apiAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // TODO: TLS
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", a.apiAddr, err)
	}
	defer conn.Close()

	client := agentv1.NewAgentServiceClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}

	// Register with the API
	hostname, _ := os.Hostname()
	if err := stream.Send(&agentv1.AgentMessage{
		Payload: &agentv1.AgentMessage_Register{
			Register: &agentv1.RegisterRequest{
				Token:    a.token,
				Hostname: hostname,
				Os:       "linux",
				Arch:     runtime.GOARCH,
				Version:  "0.1.0",
			},
		},
	}); err != nil {
		return fmt.Errorf("send register: %w", err)
	}

	// Wait for registration response
	msg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("recv register response: %w", err)
	}

	resp, ok := msg.Payload.(*agentv1.ServerMessage_RegisterResponse)
	if !ok {
		return fmt.Errorf("expected RegisterResponse, got %T", msg.Payload)
	}
	if !resp.RegisterResponse.Accepted {
		return fmt.Errorf("registration rejected: %s", resp.RegisterResponse.Message)
	}

	slog.Info("registered with API",
		"server_id", resp.RegisterResponse.ServerId,
		"hostname", hostname,
	)

	return a.session(ctx, stream)
}

// session handles the main message loop after registration.
func (a *Agent) session(ctx context.Context, stream agentv1.AgentService_ConnectClient) error {
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	recv := make(chan *agentv1.ServerMessage, 8)
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
			return fmt.Errorf("stream recv: %w", err)

		case <-heartbeat.C:
			if err := stream.Send(&agentv1.AgentMessage{
				Payload: &agentv1.AgentMessage_Heartbeat{
					Heartbeat: a.buildHeartbeat(),
				},
			}); err != nil {
				return fmt.Errorf("send heartbeat: %w", err)
			}

		case msg := <-recv:
			if cmd, ok := msg.Payload.(*agentv1.ServerMessage_Command); ok {
				go a.handleCommand(ctx, stream, cmd.Command)
			}
		}
	}
}

func (a *Agent) handleCommand(ctx context.Context, stream agentv1.AgentService_ConnectClient, cmd *agentv1.Command) {
	slog.Info("received command", "command_id", cmd.CommandId, "type", fmt.Sprintf("%T", cmd.Payload))
	result := dispatch(cmd)
	_ = stream.Send(&agentv1.AgentMessage{
		Payload: &agentv1.AgentMessage_CommandResult{CommandResult: result},
	})
}

func (a *Agent) buildHeartbeat() *agentv1.Heartbeat {
	metrics := &agentv1.SystemMetrics{}

	if cur, err := system.SampleCPU(); err == nil {
		metrics.CpuPercent = system.CPUPercent(a.lastCPU, cur)
		a.lastCPU = cur
	}
	if mem, err := system.ReadMemInfo(); err == nil {
		metrics.MemTotal = mem.Total
		metrics.MemUsed = mem.Used
	}
	if disk, err := system.ReadDiskInfo("/"); err == nil {
		metrics.DiskTotal = disk.Total
		metrics.DiskUsed = disk.Used
	}
	if load, err := system.ReadLoadAvg(); err == nil {
		metrics.Load_1 = load.Load1
		metrics.Load_5 = load.Load5
		metrics.Load_15 = load.Load15
	}

	return &agentv1.Heartbeat{
		Timestamp: timestamppb.Now(),
		Metrics:   metrics,
		Services:  []*agentv1.ServiceStatus{},
	}
}
