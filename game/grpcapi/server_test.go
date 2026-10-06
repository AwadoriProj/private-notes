package grpcapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"private-notes/game/config"
	externalpaymentspb "private-notes/game/proto/app/external_payments"
	masterdatapb "private-notes/game/proto/app/masterdata"
	playerpb "private-notes/game/proto/app/player"
	playerloginpb "private-notes/game/proto/app/playerlogin"
	presentpb "private-notes/game/proto/app/present"
)

const (
	testSdkUID   = "2104153174782697472"
	testSdkToken = "75090ff46a8d11315853240cfae9bca10041"
)

type harness struct {
	t      *testing.T
	conn   *grpc.ClientConn
	now    time.Time
	srvURL string
}

func newHarness(t *testing.T, playersPath string) *harness {
	t.Helper()
	h := &harness{t: t, now: time.Unix(1791100545, 0)}
	api, err := New(Settings{
		Servers: []config.ServerEntry{{
			ID:                16841,
			Name:              "global server",
			CDNRoot:           "https://cdn.example/prod/en_x",
			APIServerRoot:     "https://api.example",
			ChatServerRoot:    "wss://chat.example/ws",
			ATServerRoot:      "https://at.example",
			LiveServer:        "live.example:9001",
			AreaID:            "3",
			DisplayName:       "EN Region",
			AgeIconSpriteName: "",
		}},
		Version:         "11bbb30383ff4acda2173610b3c4c67e",
		ResourceVersion: "1.0.0.300",
		PlayersPath:     playersPath,
		NGWords:         []string{"rock"},
		VerifyAccessToken: func(_ context.Context, uid, token string) bool {
			return uid == testSdkUID && token == testSdkToken
		},
		Now: func() time.Time { return h.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer(api.ServerOptions()...)
	api.Register(grpcServer)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grpcServer.ServeHTTP(w, r)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	conn, err := grpc.NewClient(
		strings.TrimPrefix(srv.URL, "https://"),
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}})),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	h.conn = conn
	h.srvURL = srv.URL
	return h
}

func loginRequest() *playerloginpb.PlayerLoginRequest {
	return &playerloginpb.PlayerLoginRequest{
		SdkUid:         testSdkUID,
		SdkAccessToken: testSdkToken,
		Platform:       0,
		DeviceModel:    "OPPO CPH2269",
		ClientVersion:  "1.0.2",
		Uuid:           &playerloginpb.PlayerUuid{Identifier: "074428672b62a85a45e3a1b99f69f444"},
		ClientPackage:  "com.bilibili.sirius",
	}
}

func authCtx(id, cred string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(),
		"x-player-id", id,
		"x-player-credential", cred,
		"x-platform", "android",
		"x-client-version", "1.0.2",
	)
}

func TestFullBootstrapSequence(t *testing.T) {
	h := newHarness(t, filepath.Join(t.TempDir(), "players.json"))
	login := playerloginpb.NewPlayerLoginServiceClient(h.conn)

	var hdr metadata.MD
	list, err := login.GetServerList(context.Background(), &playerloginpb.GetServerListRequest{}, grpc.Header(&hdr))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Servers) != 1 || list.Servers[0].DisplayName != "EN Region" || list.Servers[0].AreaID != "3" {
		t.Fatalf("unexpected server list: %v", list)
	}
	if got := hdr.Get("x-asset-version"); len(got) != 1 || got[0] != "unknown" {
		t.Fatalf("x-asset-version = %v", got)
	}
	if got := hdr.Get("x-server-time"); len(got) != 1 {
		t.Fatalf("x-server-time missing: %v", hdr)
	} else if _, err := time.Parse(time.RFC3339Nano, got[0]); err != nil {
		t.Fatalf("x-server-time not RFC3339Nano: %v", err)
	}
	if len(hdr.Get("x-virtual-clock-offset")) != 0 {
		t.Fatalf("x-virtual-clock-offset must not appear on unauthenticated calls")
	}

	pre, err := login.PlayerPreLogin(context.Background(), loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	if pre.IsAccountCreated {
		t.Fatalf("new sdk uid must not be reported as created")
	}

	resp, err := login.PlayerLogin(context.Background(), loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	cred := resp.GetCredential()
	if len(cred.GetId()) != 36 || cred.GetId()[14] != '7' {
		t.Fatalf("credential id is not a uuidv7: %q", cred.GetId())
	}
	if len(cred.GetCredential()) != 32 {
		t.Fatalf("credential token length = %d", len(cred.GetCredential()))
	}
	if resp.CpServerId != "16841" || resp.CpServerName != "global server" {
		t.Fatalf("cp server = %q/%q", resp.CpServerId, resp.CpServerName)
	}
	if cred.GetDeviceId() != "" || cred.GetProfileId() != 0 {
		t.Fatalf("real server leaves device id and profile id unset in the credential")
	}

	again, err := login.PlayerLogin(context.Background(), loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(again.Credential, cred) {
		t.Fatalf("relogin must keep the same credential")
	}
	pre, err = login.PlayerPreLogin(context.Background(), loginRequest())
	if err != nil || !pre.IsAccountCreated {
		t.Fatalf("existing account must be reported as created: %v %v", pre, err)
	}

	ctx := authCtx(cred.Id, cred.Credential)
	payments := externalpaymentspb.NewExternalPaymentsServiceClient(h.conn)
	var nopHdr metadata.MD
	if _, err := payments.Nop(ctx, &emptypb.Empty{}, grpc.Header(&nopHdr)); err != nil {
		t.Fatal(err)
	}
	if got := nopHdr.Get("x-virtual-clock-offset"); len(got) != 1 || got[0] != "0" {
		t.Fatalf("x-virtual-clock-offset = %v", got)
	}

	players := playerpb.NewPlayerServiceClient(h.conn)
	data, err := players.GetPlayerData(ctx, &playerpb.GetPlayerDataRequest{})
	if err != nil {
		t.Fatal(err)
	}
	profile := data.PlayerData.MyProfile
	if profile.Name != "Player" || profile.ProfileId != data.Accountid || data.Accountid < 10_000_000_000 {
		t.Fatalf("bad profile: %v accountid=%d", profile, data.Accountid)
	}
	if len(data.PlayerData.MemberCards) != 25 || len(data.PlayerData.Stamps) != 68 {
		t.Fatalf("template content lost: cards=%d stamps=%d", len(data.PlayerData.MemberCards), len(data.PlayerData.Stamps))
	}

	presents, err := presentpb.NewPresentServiceClient(h.conn).Fetch(ctx, &presentpb.FetchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(presents.Presents) == 0 {
		t.Fatalf("no presents returned")
	}
	for _, p := range presents.Presents {
		if p.PostAt > h.now.Unix() {
			t.Fatalf("present posted in the future: %d", p.PostAt)
		}
		if p.ExpireAt != nil && *p.ExpireAt <= h.now.Unix() {
			t.Fatalf("expired present returned")
		}
	}

	var trailer metadata.MD
	_, err = players.EditProfile(ctx, &playerpb.EditProfileRequest{Name: "Playerrock"}, grpc.Trailer(&trailer))
	st, _ := status.FromError(err)
	if st.Code() != codes.Unknown || st.Message() != "player name censored" {
		t.Fatalf("censored edit: %v", err)
	}
	if got := trailer.Get("x-sirius-error-code"); len(got) != 1 || got[0] != "CENSORED" {
		t.Fatalf("x-sirius-error-code = %v (trailer %v)", got, trailer)
	}
	if _, err := players.EditProfile(ctx, &playerpb.EditProfileRequest{Name: "ROCK"}); err == nil {
		t.Fatalf("case-insensitive censor failed")
	}
	if _, err := players.EditProfile(ctx, &playerpb.EditProfileRequest{Name: "Rhadea"}); err != nil {
		t.Fatal(err)
	}
	data, err = players.GetPlayerData(ctx, &playerpb.GetPlayerDataRequest{})
	if err != nil || data.PlayerData.MyProfile.Name != "Rhadea" {
		t.Fatalf("rename not applied: %v %v", data.GetPlayerData().GetMyProfile().GetName(), err)
	}

	ver, err := masterdatapb.NewMasterdataServiceClient(h.conn).Version(ctx, &masterdatapb.VersionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if ver.Version != "11bbb30383ff4acda2173610b3c4c67e" || ver.ResourceVersion != "1.0.0.300" {
		t.Fatalf("version = %v", ver)
	}
}

func TestRejectsBadCredentialsAndSdkIdentity(t *testing.T) {
	h := newHarness(t, "")
	players := playerpb.NewPlayerServiceClient(h.conn)

	if _, err := players.GetPlayerData(context.Background(), &playerpb.GetPlayerDataRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing credential: %v", err)
	}
	if _, err := players.GetPlayerData(authCtx("nope", "nope"), &playerpb.GetPlayerDataRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong credential: %v", err)
	}

	login := playerloginpb.NewPlayerLoginServiceClient(h.conn)
	bad := loginRequest()
	bad.SdkAccessToken = "forged"
	if _, err := login.PlayerLogin(context.Background(), bad); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("forged sdk token: %v", err)
	}
	if _, err := login.PlayerLogin(context.Background(), &playerloginpb.PlayerLoginRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("empty sdk identity: %v", err)
	}

	resp, err := login.PlayerLogin(context.Background(), loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := players.GetPlayerData(authCtx(resp.Credential.Id, "wrong"), &playerpb.GetPlayerDataRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("valid id with wrong credential: %v", err)
	}
}

func TestPlayersSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "players.json")
	first := newHarness(t, path)
	login := playerloginpb.NewPlayerLoginServiceClient(first.conn)
	a, err := login.PlayerLogin(context.Background(), loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	ctx := authCtx(a.Credential.Id, a.Credential.Credential)
	if _, err := playerpb.NewPlayerServiceClient(first.conn).EditProfile(ctx, &playerpb.EditProfileRequest{Name: "Saved"}); err != nil {
		t.Fatal(err)
	}

	second := newHarness(t, path)
	b, err := playerloginpb.NewPlayerLoginServiceClient(second.conn).PlayerLogin(context.Background(), loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(a.Credential, b.Credential) {
		t.Fatalf("credential changed after restart")
	}
	data, err := playerpb.NewPlayerServiceClient(second.conn).GetPlayerData(ctx, &playerpb.GetPlayerDataRequest{})
	if err != nil || data.PlayerData.MyProfile.Name != "Saved" {
		t.Fatalf("name lost after restart: %v", err)
	}
}

func TestTemplatesMatchRealCapture(t *testing.T) {
	for name, tpl := range map[string]struct {
		raw []byte
		msg proto.Message
	}{
		"player data": {playerDataFixture, &playerpb.GetPlayerDataResponse{}},
		"presents":    {presentFetchFixture, &presentpb.FetchResponse{}},
	} {
		if err := proto.Unmarshal(tpl.raw, tpl.msg); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(tpl.msg.ProtoReflect().GetUnknown()) != 0 {
			t.Fatalf("%s: bindings do not cover the real response", name)
		}
		out, err := proto.MarshalOptions{Deterministic: true}.Marshal(tpl.msg)
		if err != nil || len(out) != len(tpl.raw) {
			t.Fatalf("%s: round trip %d vs %d (%v)", name, len(out), len(tpl.raw), err)
		}
	}
}

func TestWireLevelGzipAndTrailers(t *testing.T) {
	h := newHarness(t, "")
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
	}}
	req, err := http.NewRequest("POST", h.srvURL+"/app.playerlogin.PlayerLoginService/GetServerList", bytes.NewReader([]byte{0, 0, 0, 0, 0}))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("content-type", "application/grpc")
	req.Header.Set("te", "trailers")
	req.Header.Set("grpc-accept-encoding", "identity,gzip")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.ProtoMajor != 2 {
		t.Fatalf("not http/2: %d", resp.ProtoMajor)
	}
	if got := resp.Header.Get("grpc-encoding"); got != "gzip" {
		t.Fatalf("grpc-encoding = %q", got)
	}
	if len(body) < 5 || body[0] != 1 {
		t.Fatalf("frame is not marked compressed: % x", body[:min(len(body), 5)])
	}
	if got := resp.Trailer.Get("grpc-status"); got != "0" {
		t.Fatalf("grpc-status trailer = %q", got)
	}
	if resp.Header.Get("x-server-time") == "" || resp.Header.Get("x-asset-version") != "unknown" {
		t.Fatalf("metadata headers missing: %v", resp.Header)
	}
}
