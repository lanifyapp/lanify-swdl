package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/lanifyapp/lanify-swdl/internal/handler"
	"github.com/lanifyapp/lanify-swdl/internal/steamcmd"
	"github.com/valyala/fasthttp"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	steamCmdPath    string
	listenHost      string
	listenPort      string
	steamUser       string
	steamPassword   string
	installSteamCmd bool
	debugMode       bool
)

func init() {
	flag.StringVar(&steamCmdPath, "steamcmdpath", "steamcmd", "Path to the steamcmd directory")
	flag.BoolVar(&installSteamCmd, "installsteamcmd", true, "Install steamcmd if needed")
	flag.BoolVar(&debugMode, "debug", false, "Enable debug mode")
	flag.StringVar(&listenHost, "listenhost", "0.0.0.0", "Hostname for the server to listen on")
	flag.StringVar(&listenPort, "listenport", "8080", "Port for the server to listen on")
	flag.StringVar(&steamUser, "steamuser", "", "Steam username")
	flag.StringVar(&steamPassword, "steampassword", "", "Steam password")
}

//go:embed logo.svg
var logo []byte

func main() {
	flag.Parse()

	configureLogger(debugMode)

	slog.Info("starting lanify-swdl", "version", version, "commit", commit, "date", date)

	s, err := steamcmd.New(steamCmdPath, steamUser, steamPassword)
	if err != nil {
		fatal("steamcmd initialization failed", err)
	}

	if installSteamCmd {
		if err := os.MkdirAll(s.InstallPath, 0755); err != nil {
			fatal("failed to create steamcmd directory", err)
		}

		if err := s.Install(); err != nil {
			slog.Warn("steamcmd installation returned warning", "error", err)
		}
	}

	h, err := handler.New(s)
	if err != nil {
		fatal("handler initialization failed", err)
	}

	listenAddr := net.JoinHostPort(listenHost, listenPort)

	slog.Info(
		"server starting",
		"targets",
		formatListenTargets(listenHost, listenPort),
	)

	if err := fasthttp.ListenAndServe(listenAddr, routeRequest(h)); err != nil {
		fatal("failed to start server", err)
	}
}

func configureLogger(debug bool) {
	level := slog.LevelInfo

	if debug {
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})))
}

func fatal(message string, err error) {
	slog.Error(message, "error", err)
	os.Exit(1)
}

func routeRequest(h *handler.App) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())
		method := string(ctx.Method())

		switch {
		case method == fasthttp.MethodGet && path == "/":
			ctx.SetStatusCode(fasthttp.StatusMovedPermanently)
			ctx.Response.Header.Set("Location", "/workshop/")
		case method == fasthttp.MethodGet && (path == "/favicon.ico" || path == "/favicon.svg"):
			ctx.SetStatusCode(fasthttp.StatusOK)
			ctx.SetContentType("image/svg+xml")
			ctx.SetBody(logo)
		case method == fasthttp.MethodGet && bindTwoParams(
			ctx,
			path,
			"/api/workshop/",
			"app_id",
			"workshop_id",
		):
			h.DownloadWorkshopHandler(ctx)
		case method == fasthttp.MethodGet && bindTwoParams(
			ctx,
			path,
			"/api/collection/",
			"app_id",
			"collection_id",
		):
			h.DownloadCollectionHandler(ctx)
		case isAccountAccessRoute(path) || isUnsupportedRoute(path):
			h.UnsupportedPageHandler(ctx)
		case isProxyRoute(path):
			h.SteamProxyHandler(ctx)
		default:
			ctx.Error("Not found", fasthttp.StatusNotFound)
		}
	}
}

func bindTwoParams(ctx *fasthttp.RequestCtx, path string, prefix string, firstName string, secondName string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}

	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}

	ctx.SetUserValue(firstName, parts[0])
	ctx.SetUserValue(secondName, parts[1])
	return true
}

func isAccountAccessRoute(path string) bool {
	path = strings.ToLower(path)
	path = strings.TrimPrefix(path, "/steamcommunity")

	for _, prefix := range []string{
		"/login",
		"/join",
		"/register",
		"/account",
		"/my",
		"/id",
		"/profiles",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}

	return false
}

func isUnsupportedRoute(path string) bool {
	switch strings.ToLower(path) {
	case "/market/",
		"/discussions/":
		return true
	default:
		return false
	}
}

func isProxyRoute(path string) bool {
	for _, prefix := range []string{
		"/steamcommunity",
		"/steamstatic-community",
		"/steamstatic-cdn",
		"/steamstatic-shared",
		"/steamstatic-clan",
		"/steamstatic-avatars",
		"/steamstatic-cloudflare",
		"/steamstatic-cloudflare-alt",
		"/steamstore",
		"/steamcdn-akamai",
		"/steam-media",
		"/steam-video",
		"/steamusercontent-images",
		"/steamusercontent",
		"/workshop",
		"/app",
		"/public",
		"/sharedfiles",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func formatListenTargets(host string, port string) string {
	if host == "" {
		host = "127.0.0.1"
	}

	if host == "0.0.0.0" || host == "::" {
		return fmt.Sprintf(
			"http://127.0.0.1:%s (local), http://localhost:%s (local), %s:%s (all interfaces)",
			port,
			port,
			host,
			port,
		)
	}

	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}

	return fmt.Sprintf("http://%s:%s", host, port)
}
