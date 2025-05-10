package grpc

import (
	"context"
	"errors"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"publisher_subscriber/internal/server/grpc/config"
	"publisher_subscriber/internal/subpub"
	pubsubV1 "publisher_subscriber/protos/gen/go"
)

type Server struct {
	pubsubV1.UnimplementedPubSubServer
	pubSub     subpub.SubPub
	config     *config.Config
	grpcServer *grpc.Server
}

func New(cfg *config.Config, pubSub subpub.SubPub) *Server {
	log.Printf("Initializing gRPC server with config: %+v", cfg)
	return &Server{
		pubSub: pubSub,
		config: cfg,
	}
}

func (s *Server) Start() error {
	addr := ":" + s.config.GRPC.Port
	log.Printf("Starting gRPC server on address %s", addr)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("Failed to listen on address %s: %v", addr, err)
		return err
	}

	s.grpcServer = grpc.NewServer()
	pubsubV1.RegisterPubSubServer(s.grpcServer, s)

	log.Printf("gRPC server successfully started on port %s", s.config.GRPC.Port)

	err = s.grpcServer.Serve(lis)
	if err != nil {
		log.Printf("gRPC server failed to serve: %v", err)
		return err
	}

	return nil
}

func (s *Server) Stop() {
	if s.grpcServer != nil {
		log.Println("Shutting down gRPC server gracefully...")
		s.grpcServer.GracefulStop()
		log.Println("gRPC server shutdown completed")
	}
}

func (s *Server) Subscribe(req *pubsubV1.SubscribeRequest, stream pubsubV1.PubSub_SubscribeServer) error {
	ctx := stream.Context()
	key := req.GetKey()

	log.Printf("New subscription request for key: %s", key)

	events := make(chan string, subpub.BufferSize)
	defer func() {
		close(events)
		log.Printf("Subscription channel closed for key: %s", key)
	}()

	sub, err := s.pubSub.Subscribe(key, func(msg interface{}) {
		data, ok := msg.(string)
		if !ok {
			log.Printf("Received invalid message type for key %s: %v", key, msg)
			return
		}

		select {
		case events <- data:
			log.Printf("Message queued for key %s (size: %d/%d)", key, len(events), cap(events))
		case <-ctx.Done():
			log.Printf("Context canceled while trying to send message for key %s", key)
			return
		default:
			log.Printf("Message dropped for key %s: buffer full (size: %d)", key, len(events))
		}
	})
	if err != nil {
		log.Printf("Subscription failed for key %s: %v", key, err)
		return status.Errorf(codes.Internal, "Failed to subscribe to key %s: %v", key, err)
	}
	defer func() {
		sub.Unsubscribe()
		log.Printf("Unsubscribed from key: %s", key)
	}()

	log.Printf("Subscription active for key: %s", key)
	for {
		select {
		case data := <-events:
			log.Printf("Sending message to subscriber of key %s", key)
			err = stream.Send(
				&pubsubV1.Event{
					Data: data,
				},
			)
			if err != nil {
				log.Printf("Failed to send message to subscriber of key %s: %v", key, err)
				return status.Errorf(codes.Internal, "Failed to send event: %v", err)
			}
		case <-ctx.Done():
			log.Printf("Subscription context ended for key %s: %v", key, ctx.Err())
			return nil
		}
	}
}

func (s *Server) Publish(_ context.Context, req *pubsubV1.PublishRequest) (*emptypb.Empty, error) {
	key := req.GetKey()
	data := req.GetData()

	log.Printf("Publish request received for key %s (data size: %d)", key, len(data))

	err := s.pubSub.Publish(key, data)
	if err != nil {
		if errors.Is(err, subpub.ErrNoSubscribers) {
			log.Printf("No subscribers for key %s", key)
			return nil, status.Error(codes.NotFound, err.Error())
		}
		log.Printf("Failed to publish message for key %s: %v", key, err)
		return nil, status.Errorf(codes.Internal, "Failed to publish data: %v", err)
	}

	log.Printf("Message successfully published to key %s", key)
	return &emptypb.Empty{}, nil
}
