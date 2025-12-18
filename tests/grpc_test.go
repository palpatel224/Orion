package tests

import (
	"context"
	"net"
	"testing"

	orchestratorv1 "github.com/aditip149209/Orion/api/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type mockHealthServer struct {
	orchestratorv1.UnimplementedHealthServiceServer
}

func (s *mockHealthServer) Check(ctx context.Context, req *orchestratorv1.HealthCheckRequest) (*orchestratorv1.HealthCheckResponse, error) {
	return &orchestratorv1.HealthCheckResponse{
		Status:  "ok",
		Message: "Service " + req.Service + " is healthy",
	}, nil
}

func TestGRPCSetup(t *testing.T) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	s := grpc.NewServer()
	orchestratorv1.RegisterHealthServiceServer(s, &mockHealthServer{})

	go func() {
		if err := s.Serve(lis); err != nil {
			t.Errorf("Server exited with error: %v", err)
		}
	}()
	defer s.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	defer conn.Close()

	client := orchestratorv1.NewHealthServiceClient(conn)

	resp, err := client.Check(ctx, &orchestratorv1.HealthCheckRequest{
		Service: "test-service",
	})
	if err != nil {
		t.Fatalf("Check RPC failed: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", resp.Status)
	}

	expectedMsg := "Service test-service is healthy"
	if resp.Message != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, resp.Message)
	}

	t.Log("✓ gRPC is correctly installed and working!")
}
