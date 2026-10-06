package grpcapi

import (
	"context"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	playerloginpb "private-notes/game/proto/app/playerlogin"
	entitypb "private-notes/game/proto/entity"
)

type loginService struct {
	playerloginpb.UnimplementedPlayerLoginServiceServer
	s *Server
}

func (l *loginService) authorize(ctx context.Context, req *playerloginpb.PlayerLoginRequest) error {
	if req.GetSdkUid() == "" || req.GetSdkAccessToken() == "" {
		return status.Error(codes.Unauthenticated, "SDK identity is required")
	}
	if verify := l.s.settings.VerifyAccessToken; verify != nil && !verify(ctx, req.GetSdkUid(), req.GetSdkAccessToken()) {
		return status.Error(codes.Unauthenticated, "SDK access token rejected")
	}
	return nil
}

func (l *loginService) GetServerList(context.Context, *playerloginpb.GetServerListRequest) (*playerloginpb.GetServerListResponse, error) {
	servers := make([]*playerloginpb.ServerInfo, 0, len(l.s.settings.Servers))
	for _, entry := range l.s.settings.Servers {
		display := entry.DisplayName
		if display == "" {
			display = entry.Name
		}
		servers = append(servers, &playerloginpb.ServerInfo{
			Name:              entry.Name,
			CdnRoot:           entry.CDNRoot,
			ApiServerRoot:     entry.APIServerRoot,
			ChatServerRoot:    entry.ChatServerRoot,
			AtServerRoot:      entry.ATServerRoot,
			LiveServer:        entry.LiveServer,
			DisplayName:       display,
			AreaID:            entry.AreaID,
			AgeIconSpriteName: entry.AgeIconSpriteName,
		})
	}
	return &playerloginpb.GetServerListResponse{Servers: servers}, nil
}

func (l *loginService) PlayerPreLogin(ctx context.Context, req *playerloginpb.PlayerLoginRequest) (*playerloginpb.PlayerPreLoginResponse, error) {
	if err := l.authorize(ctx, req); err != nil {
		return nil, err
	}
	return &playerloginpb.PlayerPreLoginResponse{IsAccountCreated: l.s.players.exists(req.GetSdkUid())}, nil
}

func (l *loginService) PlayerLogin(ctx context.Context, req *playerloginpb.PlayerLoginRequest) (*playerloginpb.PlayerLoginResponse, error) {
	if err := l.authorize(ctx, req); err != nil {
		return nil, err
	}
	if len(l.s.settings.Servers) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "no game server is configured")
	}
	p, err := l.s.players.login(req.GetSdkUid(), req.GetUuid().GetIdentifier())
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create player")
	}
	server := l.s.settings.Servers[0]
	return &playerloginpb.PlayerLoginResponse{
		Credential: &entitypb.PlayerCredential{
			Id:         p.ID,
			Credential: p.Credential,
		},
		CpServerId:   strconv.Itoa(server.ID),
		CpServerName: server.Name,
	}, nil
}
