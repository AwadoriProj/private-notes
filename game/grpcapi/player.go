package grpcapi

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	playerpb "private-notes/game/proto/app/player"
)

type playerService struct {
	playerpb.UnimplementedPlayerServiceServer
	s *Server
}

func (p *playerService) GetPlayerData(ctx context.Context, _ *playerpb.GetPlayerDataRequest) (*playerpb.GetPlayerDataResponse, error) {
	current := playerFrom(ctx)
	resp := &playerpb.GetPlayerDataResponse{}
	if err := proto.Unmarshal(playerDataFixture, resp); err != nil {
		return nil, status.Error(codes.Internal, "player data template is corrupt")
	}
	resp.Accountid = current.ProfileID
	if profile := resp.GetPlayerData().GetMyProfile(); profile != nil {
		profile.Name = current.Name
		profile.ProfileId = current.ProfileID
		profile.LastUpdatedAt = current.UpdatedAt
	}
	return resp, nil
}

func (p *playerService) censored(name string) bool {
	lower := strings.ToLower(name)
	for _, word := range p.s.settings.NGWords {
		if word != "" && strings.Contains(lower, strings.ToLower(word)) {
			return true
		}
	}
	return false
}

func (p *playerService) EditProfile(ctx context.Context, req *playerpb.EditProfileRequest) (*playerpb.EditProfileResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "player name is empty")
	}
	if p.censored(name) {
		_ = grpc.SetTrailer(ctx, metadata.Pairs(headerErrorCode, "CENSORED"))
		return nil, status.Error(codes.Unknown, "player name censored")
	}
	if _, err := p.s.players.rename(playerFrom(ctx).ID, name); err != nil {
		return nil, status.Error(codes.Internal, "failed to save profile")
	}
	return &playerpb.EditProfileResponse{}, nil
}
