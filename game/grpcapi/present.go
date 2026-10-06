package grpcapi

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	presentpb "private-notes/game/proto/app/present"
)

type presentService struct {
	presentpb.UnimplementedPresentServiceServer
	s *Server
}

func (p *presentService) Fetch(ctx context.Context, _ *presentpb.FetchRequest) (*presentpb.FetchResponse, error) {
	current := playerFrom(ctx)
	resp := &presentpb.FetchResponse{}
	if err := proto.Unmarshal(presentFetchFixture, resp); err != nil {
		return nil, status.Error(codes.Internal, "present template is corrupt")
	}
	var anchor int64
	for _, item := range resp.Presents {
		if item.PostAt > anchor {
			anchor = item.PostAt
		}
	}
	delta := current.CreatedAt - anchor
	now := p.s.now().Unix()
	kept := resp.Presents[:0]
	for _, item := range resp.Presents {
		item.PostAt += delta
		if item.ExpireAt != nil {
			expire := *item.ExpireAt + delta
			if expire <= now {
				continue
			}
			item.ExpireAt = &expire
		}
		kept = append(kept, item)
	}
	resp.Presents = kept
	return resp, nil
}
