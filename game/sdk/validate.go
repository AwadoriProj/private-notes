package sdk

import (
	"context"
	"strconv"
)

func (s *Store) ValidateAccessKey(ctx context.Context, uid, key string) bool {
	entry, ok, err := s.LookupByAccessKey(ctx, key)
	if err != nil || !ok {
		return false
	}
	return uid == "" || uid == strconv.FormatInt(entry.UID, 10)
}
