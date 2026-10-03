package grpcapi

import (
	"context"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"private-notes/game/config"
	masterdatapb "private-notes/game/proto/app/masterdata"
	playerloginpb "private-notes/game/proto/app/playerlogin"
	entitypb "private-notes/game/proto/entity"
)

type Settings struct {
	Servers         []config.ServerEntry
	Version         string
	ResourceVersion string
}

type Server struct {
	playerloginpb.UnimplementedPlayerLoginServiceServer
	masterdatapb.UnimplementedMasterdataServiceServer
	settings Settings
}

func New(settings Settings) *Server {
	return &Server{settings: settings}
}

func (s *Server) Register(registrar grpc.ServiceRegistrar) {
	playerloginpb.RegisterPlayerLoginServiceServer(registrar, s)
	masterdatapb.RegisterMasterdataServiceServer(registrar, s)
}

func (s *Server) GetServerList(context.Context, *playerloginpb.GetServerListRequest) (*playerloginpb.GetServerListResponse, error) {
	servers := make([]*playerloginpb.ServerInfo, 0, len(s.settings.Servers))
	for _, entry := range s.settings.Servers {
		servers = append(servers, &playerloginpb.ServerInfo{
			Name:           entry.Name,
			CdnRoot:        entry.CDNRoot,
			ApiServerRoot:  entry.APIServerRoot,
			ChatServerRoot: entry.ChatServerRoot,
			AtServerRoot:   entry.ATServerRoot,
			LiveServer:     entry.LiveServer,
			DisplayName:    entry.Name,
			AreaID:         entry.AreaID,
		})
	}
	return &playerloginpb.GetServerListResponse{Servers: servers}, nil
}

func (s *Server) PlayerPreLogin(_ context.Context, req *playerloginpb.PlayerLoginRequest) (*playerloginpb.PlayerPreLoginResponse, error) {
	if req.GetSdkUid() == "" || req.GetSdkAccessToken() == "" {
		return nil, status.Error(codes.Unauthenticated, "SDK identity is required")
	}
	return &playerloginpb.PlayerPreLoginResponse{IsAccountCreated: true}, nil
}

func (s *Server) PlayerLogin(_ context.Context, req *playerloginpb.PlayerLoginRequest) (*playerloginpb.PlayerLoginResponse, error) {
	if req.GetSdkUid() == "" || req.GetSdkAccessToken() == "" {
		return nil, status.Error(codes.Unauthenticated, "SDK identity is required")
	}
	if len(s.settings.Servers) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "no game server is configured")
	}
	profileID, _ := strconv.ParseInt(req.GetSdkUid(), 10, 64)
	return &playerloginpb.PlayerLoginResponse{
		Credential: &entitypb.PlayerCredential{
			Id:         req.GetSdkUid(),
			Credential: req.GetSdkAccessToken(),
			DeviceId:   req.GetUuid().GetIdentifier(),
			ProfileId:  profileID,
		},
		CpServerId:   strconv.Itoa(s.settings.Servers[0].ID),
		CpServerName: s.settings.Servers[0].Name,
	}, nil
}

func (s *Server) Version(context.Context, *masterdatapb.VersionRequest) (*masterdatapb.VersionResponse, error) {
	return &masterdatapb.VersionResponse{Version: s.settings.Version, ResourceVersion: s.settings.ResourceVersion}, nil
}
