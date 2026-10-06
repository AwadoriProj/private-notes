package grpcapi

import (
	"context"

	masterdatapb "private-notes/game/proto/app/masterdata"
)

type masterdataService struct {
	masterdatapb.UnimplementedMasterdataServiceServer
	s *Server
}

func (m *masterdataService) Version(context.Context, *masterdatapb.VersionRequest) (*masterdatapb.VersionResponse, error) {
	return &masterdatapb.VersionResponse{
		Version:         m.s.settings.Version,
		ResourceVersion: m.s.settings.ResourceVersion,
	}, nil
}
