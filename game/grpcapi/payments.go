package grpcapi

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"
	externalpaymentspb "private-notes/game/proto/app/external_payments"
)

type paymentsService struct {
	externalpaymentspb.UnimplementedExternalPaymentsServiceServer
}

func (*paymentsService) Nop(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
