package grpcapi

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	headerPlayerID   = "x-player-id"
	headerCredential = "x-player-credential"
	headerErrorCode  = "x-sirius-error-code"
)

var openMethods = map[string]bool{
	"/app.playerlogin.PlayerLoginService/GetServerList":  true,
	"/app.playerlogin.PlayerLoginService/PlayerPreLogin": true,
	"/app.playerlogin.PlayerLoginService/PlayerLogin":    true,
}

type playerKey struct{}

func playerFrom(ctx context.Context) player {
	p, _ := ctx.Value(playerKey{}).(player)
	return p
}

func firstValue(md metadata.MD, key string) string {
	if v := md.Get(key); len(v) > 0 {
		return v[0]
	}
	return ""
}

func (s *Server) unaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	authenticated := false
	if !openMethods[info.FullMethod] {
		p, ok := s.players.authenticate(firstValue(md, headerPlayerID), firstValue(md, headerCredential))
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "invalid player credential")
		}
		ctx = context.WithValue(ctx, playerKey{}, p)
		authenticated = true
	}
	resp, err := handler(ctx, req)
	if err != nil {
		return nil, err
	}
	header := metadata.Pairs(
		"x-asset-version", "unknown",
		"x-server-time", s.now().UTC().Format(time.RFC3339Nano),
	)
	if authenticated {
		header.Append("x-virtual-clock-offset", "0")
	}
	_ = grpc.SetHeader(ctx, header)
	return resp, nil
}

func (s *Server) ServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.UnaryInterceptor(s.unaryInterceptor),
		grpc.RPCCompressor(grpc.NewGZIPCompressor()),
	}
}
