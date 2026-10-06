package grpcapi

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"private-notes/game/config"
	externalpaymentspb "private-notes/game/proto/app/external_payments"
	masterdatapb "private-notes/game/proto/app/masterdata"
	playerpb "private-notes/game/proto/app/player"
	playerloginpb "private-notes/game/proto/app/playerlogin"
	presentpb "private-notes/game/proto/app/present"
)

type Settings struct {
	Servers           []config.ServerEntry
	Version           string
	ResourceVersion   string
	PlayersPath       string
	NGWords           []string
	VerifyAccessToken func(ctx context.Context, uid, token string) bool
	Now               func() time.Time
}

type Server struct {
	settings Settings
	players  *playerStore
	now      func() time.Time
}

func New(settings Settings) (*Server, error) {
	now := settings.Now
	if now == nil {
		now = time.Now
	}
	players, err := newPlayerStore(settings.PlayersPath, now)
	if err != nil {
		return nil, err
	}
	return &Server{settings: settings, players: players, now: now}, nil
}

func (s *Server) Register(registrar grpc.ServiceRegistrar) {
	playerloginpb.RegisterPlayerLoginServiceServer(registrar, &loginService{s: s})
	masterdatapb.RegisterMasterdataServiceServer(registrar, &masterdataService{s: s})
	playerpb.RegisterPlayerServiceServer(registrar, &playerService{s: s})
	presentpb.RegisterPresentServiceServer(registrar, &presentService{s: s})
	externalpaymentspb.RegisterExternalPaymentsServiceServer(registrar, &paymentsService{})
}
