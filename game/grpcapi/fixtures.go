package grpcapi

import _ "embed"

//go:embed fixtures/get_player_data.bin
var playerDataFixture []byte

//go:embed fixtures/present_fetch.bin
var presentFetchFixture []byte
