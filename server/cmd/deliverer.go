package cmd

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/THPTUHA/kairos/server/deliverer/api"
	"github.com/THPTUHA/kairos/server/deliverer/client"
	"github.com/THPTUHA/kairos/server/deliverer/jwtverify"
	"github.com/THPTUHA/kairos/server/deliverer/notify"
	"github.com/THPTUHA/kairos/server/deliverer/rule"
	"github.com/THPTUHA/kairos/server/deliverer/survey"
	"github.com/THPTUHA/kairos/server/deliverer/tools"
	"github.com/THPTUHA/kairos/server/deliverer/usage"
	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

const (
	engineName          = "memory"
	granularProxyMode   = false
	useAPIOpentelemetry = false
	grpcAPIAddress      = ""
	grpcAPIPort         = 10000
	grpcAPIKey          = ""
	grpcAPITls          = false
	grpcAPITlsDisable   = false
	usageStatsDisable   = false
	brokerName          = ""
	configFile          = ""
)

func init() {
	kairosCmd.AddCommand(delivererCmd)
}

const transportErrorMode = "transport"

var delivererCmd = &cobra.Command{
	Use:   "deliver",
	Short: "Run deliverer for deliver message",
	Long:  `Run deliverer for deliver message`,
	Run: func(cmd *cobra.Command, args []string) {
		ruleConfig := ruleConfig()
		err := ruleConfig.Validate()

		if err != nil {
			log.Fatal().Msgf("error validating config: %v", err)
		}

		ruleContainer, err := rule.NewContainer(ruleConfig)
		if err != nil {
			log.Fatal().Msgf("error creating config: %v", err)
		}

		var proxyMap *client.ProxyMap
		var proxyEnabled bool
		proxyMap, proxyEnabled = proxyMapConfig()

		nodeCfg := nodeConfig()
		node, err := centrifuge.New(nodeCfg)

		if err != nil {
			log.Fatal().Msgf("error creating Deliverer Node: %v", err)
		}

		var broker centrifuge.Broker
		var presenceManager centrifuge.PresenceManager

		var engineMode string
		if engineName == "memory" {
			broker, presenceManager, engineMode, err = memoryEngine(node)
		}

		if err != nil {
			log.Fatal().Msgf("error creating engine: %v", err)
		}

		tokenVerifier, err := jwtverify.NewTokenVerifierJWT(jwtVerifierConfig(), ruleContainer)
		if err != nil {
			log.Fatal().Msgf("error creating token verifier: %v", err)
		}

		var subTokenVerifier *jwtverify.VerifierJWT
		clientHandler := client.NewHandler(node, ruleContainer, tokenVerifier, subTokenVerifier, proxyMap, granularProxyMode)
		err = clientHandler.Setup()
		if err != nil {
			log.Fatal().Msgf("error setting up client handler: %v", err)
		}

		surveyCaller := survey.NewCaller(node)

		grpcAPIExecutor := api.NewExecutor(node, ruleContainer, surveyCaller, "grpc", useAPIOpentelemetry)

		node.SetBroker(broker)
		node.SetPresenceManager(presenceManager)

		var disableHistoryPresence bool
		if engineName == "memory" {
			// Presence and History won't work with Memory engine in distributed case.
			disableHistoryPresence = true
			node.SetPresenceManager(nil)
		}

		if disableHistoryPresence {
			log.Warn().Msgf("presence, history and recovery disabled with Memory engine and Nats broker")
		}

		if err = node.Run(); err != nil {
			log.Fatal().Msgf("error running node: %v", err)
		}

		var grpcAPIServer *grpc.Server
		var grpcAPIAddr string

		grpcAPIAddr = net.JoinHostPort(viper.GetString("grpc_api_address"), viper.GetString("grpc_api_port"))
		grpcAPIConn, err := net.Listen("tcp", grpcAPIAddr)
		if err != nil {
			log.Fatal().Msgf("cannot listen to address %s", grpcAPIAddr)
		}
		var grpcOpts []grpc.ServerOption
		var tlsConfig *tls.Config
		var tlsErr error

		if grpcAPIKey != "" {
			grpcOpts = append(grpcOpts, api.GRPCKeyAuth(viper.GetString("grpc_api_key")))
		}
		if grpcAPITls {
			tlsConfig, tlsErr = tlsConfigForGRPC()
		} else if !grpcAPITlsDisable {
			tlsConfig, tlsErr = getTLSConfig()
		}
		if tlsErr != nil {
			log.Fatal().Msgf("error getting TLS config: %v", tlsErr)
		}
		if tlsConfig != nil {
			grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsConfig)))
		}
		if useAPIOpentelemetry {
			grpcOpts = append(grpcOpts, grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()))
		}
		grpcErrorMode, err := tools.OptionalStringChoice(viper.GetViper(), "grpc_api_error_mode", []string{transportErrorMode})
		if err != nil {
			log.Fatal().Msgf("error in config: %v", err)
		}
		grpcAPIServer = grpc.NewServer(grpcOpts...)
		_ = api.RegisterGRPCServerAPI(node, grpcAPIExecutor, grpcAPIServer, api.GRPCAPIServiceConfig{
			UseOpenTelemetry:      useAPIOpentelemetry,
			UseTransportErrorMode: grpcErrorMode == transportErrorMode,
		})
		if viper.GetBool("grpc_api_reflection") {
			reflection.Register(grpcAPIServer)
		}
		go func() {
			if err := grpcAPIServer.Serve(grpcAPIConn); err != nil {
				log.Fatal().Msgf("serve GRPC API: %v", err)
			}
		}()

		if grpcAPIServer != nil {
			log.Info().Msgf("serving GRPC API service on %s", grpcAPIAddr)
		}

		var statsSender *usage.Sender
		if !usageStatsDisable {
			statsSender = usage.NewSender(node, ruleContainer, usage.Features{
				Edition:    "oss",
				Version:    "0.0.0",
				Engine:     engineName,
				EngineMode: engineMode,
				Broker:     brokerName,
				BrokerMode: "",

				Websocket:     !viper.GetBool("websocket_disable"),
				HTTPStream:    viper.GetBool("http_stream"),
				SSE:           viper.GetBool("sse"),
				SockJS:        viper.GetBool("sockjs"),
				UniWebsocket:  viper.GetBool("uni_websocket"),
				UniHTTPStream: viper.GetBool("uni_http_stream"),
				UniSSE:        viper.GetBool("uni_sse"),
				UniGRPC:       viper.GetBool("uni_grpc"),

				GrpcAPI:             viper.GetBool("grpc_api"),
				SubscribeToPersonal: viper.GetBool("user_subscribe_to_personal"),
				Admin:               viper.GetBool("admin"),

				ConnectProxy:         proxyMap.ConnectProxy != nil,
				RefreshProxy:         proxyMap.RefreshProxy != nil,
				SubscribeProxy:       len(proxyMap.SubscribeProxies) > 0,
				PublishProxy:         len(proxyMap.PublishProxies) > 0,
				RPCProxy:             len(proxyMap.RpcProxies) > 0,
				SubRefreshProxy:      len(proxyMap.SubRefreshProxies) > 0,
				SubscribeStreamProxy: len(proxyMap.SubscribeStreamProxies) > 0,

				ClickhouseAnalytics: false,
				UserStatus:          false,
				Throttling:          false,
				Singleflight:        false,
			})
			go statsSender.Start(context.Background())
		}

		notify.RegisterHandlers(node, statsSender)
		handleDelivererSignals(configFile, node, ruleContainer, tokenVerifier, subTokenVerifier, servers, grpcAPIServer, grpcUniServer, exporter)
	},
}

func tlsConfigForGRPC() (*tls.Config, error) {
	return tools.MakeTLSConfig(viper.GetViper(), "grpc_api_")
}

func getTLSConfig() (*tls.Config, error) {
	tlsEnabled := viper.GetBool("tls")
	tlsAutocertEnabled := viper.GetBool("tls_autocert")
	autocertHostWhitelist := viper.GetString("tls_autocert_host_whitelist")
	var tlsAutocertHostWhitelist []string
	if autocertHostWhitelist != "" {
		tlsAutocertHostWhitelist = strings.Split(autocertHostWhitelist, ",")
	} else {
		tlsAutocertHostWhitelist = nil
	}
	tlsAutocertCacheDir := viper.GetString("tls_autocert_cache_dir")
	tlsAutocertEmail := viper.GetString("tls_autocert_email")
	tlsAutocertServerName := viper.GetString("tls_autocert_server_name")
	tlsAutocertHTTP := viper.GetBool("tls_autocert_http")
	tlsAutocertHTTPAddr := viper.GetString("tls_autocert_http_addr")

	if tlsAutocertEnabled {
		certManager := autocert.Manager{
			Prompt: autocert.AcceptTOS,
			Email:  tlsAutocertEmail,
		}
		if tlsAutocertHostWhitelist != nil {
			certManager.HostPolicy = autocert.HostWhitelist(tlsAutocertHostWhitelist...)
		}
		if tlsAutocertCacheDir != "" {
			certManager.Cache = autocert.DirCache(tlsAutocertCacheDir)
		}

		if tlsAutocertHTTP {
			startHTTPChallengeServerOnce.Do(func() {
				// getTLSConfig can be called several times.
				acmeHTTPServer := &http.Server{
					Handler:  certManager.HTTPHandler(nil),
					Addr:     tlsAutocertHTTPAddr,
					ErrorLog: stdlog.New(&httpErrorLogWriter{log.Logger}, "", 0),
				}
				go func() {
					log.Info().Msgf("serving ACME http_01 challenge on %s", tlsAutocertHTTPAddr)
					if err := acmeHTTPServer.ListenAndServe(); err != nil {
						log.Fatal().Msgf("can't create server on %s to serve acme http challenge: %v", tlsAutocertHTTPAddr, err)
					}
				}()
			})
		}

		return &tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				// See https://github.com/centrifugal/centrifugo/issues/144#issuecomment-279393819
				if tlsAutocertServerName != "" && hello.ServerName == "" {
					hello.ServerName = tlsAutocertServerName
				}
				return certManager.GetCertificate(hello)
			},
			NextProtos: []string{
				"h2", "http/1.1", acme.ALPNProto,
			},
		}, nil

	} else if tlsEnabled {
		// Autocert disabled - just try to use provided SSL cert and key files.
		return tools.MakeTLSConfig(viper.GetViper(), "")
	}

	return nil, nil
}

func jwtVerifierConfig() jwtverify.VerifierConfig {
	v := viper.GetViper()
	cfg := jwtverify.VerifierConfig{}

	cfg.HMACSecretKey = v.GetString("token_hmac_secret_key")

	rsaPublicKey := v.GetString("token_rsa_public_key")
	if rsaPublicKey != "" {
		pubKey, err := jwtutils.ParseRSAPublicKeyFromPEM([]byte(rsaPublicKey))
		if err != nil {
			log.Fatal().Msgf("error parsing RSA public key: %v", err)
		}
		cfg.RSAPublicKey = pubKey
	}

	ecdsaPublicKey := v.GetString("token_ecdsa_public_key")
	if ecdsaPublicKey != "" {
		pubKey, err := jwtutils.ParseECDSAPublicKeyFromPEM([]byte(ecdsaPublicKey))
		if err != nil {
			log.Fatal().Msgf("error parsing ECDSA public key: %v", err)
		}
		cfg.ECDSAPublicKey = pubKey
	}

	cfg.JWKSPublicEndpoint = v.GetString("token_jwks_public_endpoint")
	cfg.Audience = v.GetString("token_audience")
	cfg.AudienceRegex = v.GetString("token_audience_regex")
	cfg.Issuer = v.GetString("token_issuer")
	cfg.IssuerRegex = v.GetString("token_issuer_regex")
	return cfg
}

func memoryEngine(n *centrifuge.Node) (centrifuge.Broker, centrifuge.PresenceManager, string, error) {
	brokerConf, err := memoryBrokerConfig()
	if err != nil {
		return nil, nil, "", err
	}
	broker, err := centrifuge.NewMemoryBroker(n, *brokerConf)
	if err != nil {
		return nil, nil, "", err
	}
	presenceManagerConf, err := memoryPresenceManagerConfig()
	if err != nil {
		return nil, nil, "", err
	}
	presenceManager, err := centrifuge.NewMemoryPresenceManager(n, *presenceManagerConf)
	if err != nil {
		return nil, nil, "", err
	}
	return broker, presenceManager, "", nil
}

func memoryPresenceManagerConfig() (*centrifuge.MemoryPresenceManagerConfig, error) {
	return &centrifuge.MemoryPresenceManagerConfig{}, nil
}

func memoryBrokerConfig() (*centrifuge.MemoryBrokerConfig, error) {
	return &centrifuge.MemoryBrokerConfig{}, nil
}

func ruleConfig() rule.Config {
	cfg := rule.Config{}
	return cfg
}

func nodeConfig() centrifuge.Config {
	cfg := centrifuge.Config{}
	return cfg
}

func proxyMapConfig() (*client.ProxyMap, bool) {
	v := viper.GetViper()

	proxyMap := &client.ProxyMap{
		SubscribeProxies:       map[string]proxy.SubscribeProxy{},
		PublishProxies:         map[string]proxy.PublishProxy{},
		RpcProxies:             map[string]proxy.RPCProxy{},
		SubRefreshProxies:      map[string]proxy.SubRefreshProxy{},
		SubscribeStreamProxies: map[string]*proxy.SubscribeStreamProxy{},
	}

	proxyConfig := proxy.Config{
		BinaryEncoding:        v.GetBool("proxy_binary_encoding"),
		IncludeConnectionMeta: v.GetBool("proxy_include_connection_meta"),
		GrpcCertFile:          v.GetString("proxy_grpc_cert_file"),
		GrpcCredentialsKey:    v.GetString("proxy_grpc_credentials_key"),
		GrpcCredentialsValue:  v.GetString("proxy_grpc_credentials_value"),
		GrpcMetadata:          v.GetStringSlice("proxy_grpc_metadata"),
		GrpcCompression:       v.GetBool("proxy_grpc_compression"),
	}

	proxyConfig.HttpHeaders = v.GetStringSlice("proxy_http_headers")
	for i, header := range proxyConfig.HttpHeaders {
		proxyConfig.HttpHeaders[i] = strings.ToLower(header)
	}

	staticHttpHeaders, err := tools.MapStringString(v, "proxy_static_http_headers")
	if err != nil {
		log.Fatal().Err(err).Msg("malformed configuration for proxy_static_http_headers")
	}
	proxyConfig.StaticHttpHeaders = staticHttpHeaders

	connectEndpoint := v.GetString("proxy_connect_endpoint")
	connectTimeout := GetDuration("proxy_connect_timeout")
	refreshEndpoint := v.GetString("proxy_refresh_endpoint")
	refreshTimeout := GetDuration("proxy_refresh_timeout")
	rpcEndpoint := v.GetString("proxy_rpc_endpoint")
	rpcTimeout := GetDuration("proxy_rpc_timeout")
	subscribeEndpoint := v.GetString("proxy_subscribe_endpoint")
	subscribeTimeout := GetDuration("proxy_subscribe_timeout")
	publishEndpoint := v.GetString("proxy_publish_endpoint")
	publishTimeout := GetDuration("proxy_publish_timeout")
	subRefreshEndpoint := v.GetString("proxy_sub_refresh_endpoint")
	subRefreshTimeout := GetDuration("proxy_sub_refresh_timeout")
	proxyStreamSubscribeEndpoint := v.GetString("proxy_subscribe_stream_endpoint")
	if strings.HasPrefix(proxyStreamSubscribeEndpoint, "http") {
		log.Fatal().Msg("error creating subscribe stream proxy: only GRPC endpoints supported")
	}
	proxyStreamSubscribeTimeout := GetDuration("proxy_subscribe_stream_timeout")

	if connectEndpoint != "" {
		proxyConfig.Endpoint = connectEndpoint
		proxyConfig.Timeout = tools.Duration(connectTimeout)
		var err error
		proxyMap.ConnectProxy, err = proxy.GetConnectProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating connect proxy: %v", err)
		}
		log.Info().Str("endpoint", connectEndpoint).Msg("connect proxy enabled")
	}

	if refreshEndpoint != "" {
		proxyConfig.Endpoint = refreshEndpoint
		proxyConfig.Timeout = tools.Duration(refreshTimeout)
		var err error
		proxyMap.RefreshProxy, err = proxy.GetRefreshProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating refresh proxy: %v", err)
		}
		log.Info().Str("endpoint", refreshEndpoint).Msg("refresh proxy enabled")
	}

	if subscribeEndpoint != "" {
		proxyConfig.Endpoint = subscribeEndpoint
		proxyConfig.Timeout = tools.Duration(subscribeTimeout)
		sp, err := proxy.GetSubscribeProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe proxy: %v", err)
		}
		proxyMap.SubscribeProxies[""] = sp
		log.Info().Str("endpoint", subscribeEndpoint).Msg("subscribe proxy enabled")
	}

	if publishEndpoint != "" {
		proxyConfig.Endpoint = publishEndpoint
		proxyConfig.Timeout = tools.Duration(publishTimeout)
		pp, err := proxy.GetPublishProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating publish proxy: %v", err)
		}
		proxyMap.PublishProxies[""] = pp
		log.Info().Str("endpoint", publishEndpoint).Msg("publish proxy enabled")
	}

	if rpcEndpoint != "" {
		proxyConfig.Endpoint = rpcEndpoint
		proxyConfig.Timeout = tools.Duration(rpcTimeout)
		rp, err := proxy.GetRpcProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating rpc proxy: %v", err)
		}
		proxyMap.RpcProxies[""] = rp
		log.Info().Str("endpoint", rpcEndpoint).Msg("RPC proxy enabled")
	}

	if subRefreshEndpoint != "" {
		proxyConfig.Endpoint = subRefreshEndpoint
		proxyConfig.Timeout = tools.Duration(subRefreshTimeout)
		srp, err := proxy.GetSubRefreshProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating sub refresh proxy: %v", err)
		}
		proxyMap.SubRefreshProxies[""] = srp
		log.Info().Str("endpoint", subRefreshEndpoint).Msg("sub refresh proxy enabled")
	}

	if proxyStreamSubscribeEndpoint != "" {
		proxyConfig.Endpoint = proxyStreamSubscribeEndpoint
		proxyConfig.Timeout = tools.Duration(proxyStreamSubscribeTimeout)
		streamProxy, err := proxy.NewSubscribeStreamProxy(proxyConfig)
		if err != nil {
			log.Fatal().Msgf("error creating subscribe stream proxy: %v", err)
		}
		proxyMap.SubscribeStreamProxies[""] = streamProxy
		log.Info().Str("endpoint", proxyStreamSubscribeEndpoint).Msg("subscribe stream proxy enabled")
	}

	proxyEnabled := connectEndpoint != "" || refreshEndpoint != "" ||
		rpcEndpoint != "" || subscribeEndpoint != "" || publishEndpoint != "" ||
		subRefreshEndpoint != "" || proxyStreamSubscribeEndpoint != ""

	return proxyMap, proxyEnabled
}

func GetDuration(key string, secondsPrecision ...bool) time.Duration {
	durationString := viper.GetString(key)
	duration, err := time.ParseDuration(durationString)
	if err != nil {
		log.Fatal().Msgf("malformed duration for key '%s': %v", key, err)
	}
	if duration > 0 && duration < time.Millisecond {
		log.Fatal().Msgf("malformed duration for key '%s': %s, minimal duration resolution is 1ms – make sure correct time unit set", key, duration)
	}
	if duration > 0 && duration < time.Second && len(secondsPrecision) > 0 && secondsPrecision[0] {
		log.Fatal().Msgf("malformed duration for key '%s': %s, minimal duration resolution is 1s for this key", key, duration)
	}
	if duration > 0 && duration%time.Second != 0 && len(secondsPrecision) > 0 && secondsPrecision[0] {
		log.Fatal().Msgf("malformed duration for key '%s': %s, sub-second precision is not supported for this key", key, duration)
	}
	return duration
}

func handleDelivererSignals(configFile string, n *centrifuge.Node, ruleContainer *rule.Container, tokenVerifier *jwtverify.VerifierJWT, subTokenVerifier *jwtverify.VerifierJWT, grpcAPIServer *grpc.Server) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, os.Interrupt, syscall.SIGTERM)
	for {
		sig := <-sigCh
		log.Info().Msgf("signal received: %v", sig)
		switch sig {
		case syscall.SIGHUP:
			// reload application configuration on SIGHUP.
			log.Info().Msg("reloading configuration")
			err := validateConfig(configFile)
			if err != nil {
				log.Error().Msg(tools.ErrorMessageFromConfigError(err, configFile))
				continue
			}
			ruleConfig := ruleConfig()
			if err := tokenVerifier.Reload(jwtVerifierConfig()); err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			if subTokenVerifier != nil {
				if err := subTokenVerifier.Reload(subJWTVerifierConfig()); err != nil {
					log.Error().Msgf("error reloading: %v", err)
					continue
				}
			}
			if err := ruleContainer.Reload(ruleConfig); err != nil {
				log.Error().Msgf("error reloading: %v", err)
				continue
			}
			log.Info().Msg("configuration successfully reloaded")
		case syscall.SIGINT, os.Interrupt, syscall.SIGTERM:
			log.Info().Msg("shutting down ...")
			pidFile := viper.GetString("pid_file")
			shutdownTimeout := GetDuration("shutdown_timeout")
			go time.AfterFunc(shutdownTimeout, func() {
				if pidFile != "" {
					_ = os.Remove(pidFile)
				}
				os.Exit(1)
			})

			if exporter != nil {
				_ = exporter.Close()
			}

			var wg sync.WaitGroup

			if grpcAPIServer != nil {
				wg.Add(1)
				go func() {
					defer wg.Done()
					grpcAPIServer.GracefulStop()
				}()
			}

			if grpcUniServer != nil {
				wg.Add(1)
				go func() {
					defer wg.Done()
					grpcUniServer.GracefulStop()
				}()
			}

			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)

			for _, srv := range httpServers {
				wg.Add(1)
				go func(srv *http.Server) {
					defer wg.Done()
					_ = srv.Shutdown(ctx)
				}(srv)
			}

			_ = n.Shutdown(ctx)

			wg.Wait()
			cancel()

			if pidFile != "" {
				_ = os.Remove(pidFile)
			}
			time.Sleep(GetDuration("shutdown_termination_delay"))
			os.Exit(0)
		}
	}
}
