package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"private-notes/game/config"
	"private-notes/game/db"
	"private-notes/game/grpcapi"
	"private-notes/game/logging"
	"private-notes/game/sdk"
)

type serverConfig struct {
	ListenAddr      string               `json:"listen_addr"`
	TLSCert         string               `json:"tls_cert"`
	TLSKey          string               `json:"tls_key"`
	RSAKeyPath      string               `json:"rsa_key_path"`
	DBDSN           string               `json:"db_dsn"`
	Servers         []config.ServerEntry `json:"servers"`
	Version         string               `json:"version"`
	ResourceVersion string               `json:"resource_version"`
	PlayersPath     string               `json:"players_path"`
	NGWords         []string             `json:"ng_words"`
}

func loadConfig(path string) (*serverConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg serverConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func main() {
	cfgPath := "game/config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("failed to load %s: %v", cfgPath, err)
	}

	keys, err := sdk.LoadOrCreateRSAKeys(cfg.RSAKeyPath)
	if err != nil {
		log.Fatalf("rsa key setup failed: %v", err)
	}

	var database *db.Store
	if cfg.DBDSN != "" {
		database, err = db.Connect(cfg.DBDSN)
		if err != nil {
			log.Fatalf("db connect failed: %v", err)
		}
		defer database.Close()
		log.Print("connected to postgres, sessions will persist across restarts")
	} else {
		log.Print("no db_dsn set, sessions are in-memory only")
	}

	login := sdk.NewServer(keys, database)
	stubs := config.New(cfg.Servers)

	mux := http.NewServeMux()

	mux.HandleFunc("/gapi/client/activate", stubs.Activate)
	mux.HandleFunc("/gapi/client/config", stubs.Config)
	mux.HandleFunc("/gapi/client/country/list", stubs.CountryList)
	mux.HandleFunc("/gapi/client/rsa_public", login.RSAPublic)
	mux.HandleFunc("/gapi/client/mail/otp/send", login.OTPSend)
	mux.HandleFunc("/gapi/client/mail/otp/verify/login", login.OTPVerifyLogin)
	mux.HandleFunc("/gapi/client/mail/otp/verify/register", login.OTPVerifyRegister)
	mux.HandleFunc("/gapi/client/cache.login", login.CacheLogin)
	mux.HandleFunc("/gapi/client/create.role", login.CreateRole)
	mux.HandleFunc("/gapi/client/notify.zone", login.NotifyZone)
	mux.HandleFunc("/gapi/client/server/list", stubs.ServerList)

	mux.HandleFunc("/gapi/client/configV2", stubs.GameSupportConfig)
	mux.HandleFunc("/gapi/client/sync_agreement_status", stubs.SyncAgreementStatus)
	mux.HandleFunc("/netcheck/config/safe", stubs.NetcheckSafe)
	mux.HandleFunc("/app/time/conf", stubs.RealtimeConf)
	mux.HandleFunc("/app/global/time/heartbeat", stubs.RealtimeHeartbeat)

	mux.HandleFunc("/sdk/overseas/config", stubs.OverseasConfig)
	mux.HandleFunc("/sdk/overseas/notice/list", stubs.NoticeList)
	mux.HandleFunc("/sdk/login/ui/abTest", stubs.ABTest)

	mux.HandleFunc("/sdk-hot-deploy/featureFlag/client/config", stubs.FeatureFlag)
	mux.HandleFunc("/config/getConfig", stubs.CloudStorageConfig)

	mux.HandleFunc("/", logging.Unmapped)

	handler := logging.Middleware(mux)
	api, err := grpcapi.New(grpcapi.Settings{
		Servers:           cfg.Servers,
		Version:           cfg.Version,
		ResourceVersion:   cfg.ResourceVersion,
		PlayersPath:       cfg.PlayersPath,
		NGWords:           cfg.NGWords,
		VerifyAccessToken: login.Store.ValidateAccessKey,
	})
	if err != nil {
		log.Fatalf("grpc api setup failed: %v", err)
	}
	grpcServer := grpc.NewServer(api.ServerOptions()...)
	api.Register(grpcServer)
	combinedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	})

	log.Printf("private-notes listening on %s", cfg.ListenAddr)
	if cfg.TLSCert != "" {
		server := &http.Server{Addr: cfg.ListenAddr, Handler: combinedHandler}
		if err := http2.ConfigureServer(server, &http2.Server{}); err != nil {
			log.Fatalf("http/2 setup failed: %v", err)
		}
		log.Fatal(server.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey))
	} else {
		log.Fatal(http.ListenAndServe(cfg.ListenAddr, combinedHandler))
	}
}
