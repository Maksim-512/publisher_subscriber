package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	myGRPC "publisher_subscriber/internal/server/grpc"
	"publisher_subscriber/internal/server/grpc/config"
	"publisher_subscriber/internal/subpub"
	pubsubV1 "publisher_subscriber/protos/gen/go"
)

func TestPubSubService(t *testing.T) {
	port := "50052"

	cfg := &config.Config{
		GRPC: config.GRPCConfig{
			Port: port,
		},
	}
	pubSub := subpub.NewSubPub()
	server := myGRPC.New(cfg, pubSub)

	go func() {
		if err := server.Start(); err != nil {
			t.Errorf("server failed: %v", err)
		}
	}()
	defer server.Stop()

	time.Sleep(500 * time.Millisecond)

	conn, err := grpc.Dial(
		"localhost:"+port,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(2*time.Second),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := pubsubV1.NewPubSubClient(conn)

	ready := make(chan struct{})
	received := make(chan string, 1)

	go func() {
		stream, err := client.Subscribe(context.Background(), &pubsubV1.SubscribeRequest{
			Key: "test_vk",
		})
		require.NoError(t, err)

		close(ready)

		msg, err := stream.Recv()
		require.NoError(t, err)
		received <- msg.GetData()
	}()

	<-ready
	time.Sleep(100 * time.Millisecond)

	_, err = client.Publish(context.Background(), &pubsubV1.PublishRequest{
		Key:  "test_vk",
		Data: "test_message",
	})
	require.NoError(t, err)

	select {
	case msg := <-received:
		require.Equal(t, "test_message", msg)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}
