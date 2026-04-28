package handler

import (
	"bytes"
	"compress/gzip"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanifyapp/lanify-swdl/internal/steamcmd"
	"github.com/valyala/fasthttp"
)

func TestNewUsesPersistentDownloadDirectory(t *testing.T) {
	root := t.TempDir()

	s, err := steamcmd.New(root, "", "")
	if err != nil {
		t.Fatalf("steamcmd.New() error = %v", err)
	}

	h, err := New(s)
	if err != nil {
		t.Fatalf("handler.New() error = %v", err)
	}

	want := filepath.Join(root, "downloads")
	if h.saveDirectory != want {
		t.Fatalf("saveDirectory = %q, want %q", h.saveDirectory, want)
	}
}

func TestRewriteProxiedURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "steam community sharedfiles url keeps first class route",
			in:   "https://steamcommunity.com/sharedfiles/filedetails/?id=123",
			want: "/sharedfiles/filedetails/?id=123",
		},
		{
			name: "steam community workshop url keeps first class route",
			in:   "https://steamcommunity.com/workshop/filedetails/?id=123",
			want: "/workshop/filedetails/?id=123",
		},
		{
			name: "steam community non first class url remains namespaced",
			in:   "https://steamcommunity.com/actions/SearchApps/test",
			want: "/steamcommunity/actions/SearchApps/test",
		},
		{
			name: "wrapped top location",
			in:   "top.location.href='https://store.steampowered.com/app/10'",
			want: "top.location.href='/steamstore/app/10'",
		},
		{
			name: "relative path unchanged",
			in:   "/sharedfiles/filedetails/?id=123",
			want: "/sharedfiles/filedetails/?id=123",
		},
		{
			name: "disallowed host unchanged",
			in:   "https://example.com/file.js",
			want: "https://example.com/file.js",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := rewriteProxiedURL(tc.in); got != tc.want {
				t.Fatalf("rewriteProxiedURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRewriteTextContentKeepsFirstClassSteamCommunityRoutes(t *testing.T) {
	body := []byte(`href="https://steamcommunity.com/workshop/filedetails/?id=1";src="https:\/\/steamcommunity.com\/sharedfiles\/filedetails\/?id=2";href="https://steamcommunity.com/actions/SearchApps/test"`)

	rewritten := string(rewriteTextContent(body))

	for _, want := range []string{
		`href="/workshop/filedetails/?id=1"`,
		`src="\/sharedfiles\/filedetails\/?id=2"`,
		`href="/steamcommunity/actions/SearchApps/test"`,
	} {
		if !strings.Contains(rewritten, want) {
			t.Fatalf("rewritten text missing %q:\n%s", want, rewritten)
		}
	}
}

func TestResolveUpstreamSupportsFirstClassSteamCommunityRoutes(t *testing.T) {
	tests := []struct {
		path       string
		wantPrefix string
		wantBase   string
	}{
		{path: "/workshop/filedetails/", wantPrefix: "", wantBase: "https://steamcommunity.com"},
		{path: "/app/4000", wantPrefix: "", wantBase: "https://steamcommunity.com"},
		{path: "/public/shared.css", wantPrefix: "", wantBase: "https://steamcommunity.com"},
		{path: "/sharedfiles/filedetails/", wantPrefix: "", wantBase: "https://steamcommunity.com"},
		{path: "/steamcommunity/actions/SearchApps/test", wantPrefix: "/steamcommunity", wantBase: "https://steamcommunity.com"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			prefix, base, ok := resolveUpstream(tc.path)
			if !ok {
				t.Fatalf("resolveUpstream(%q) ok = false, want true", tc.path)
			}
			if prefix != tc.wantPrefix || base != tc.wantBase {
				t.Fatalf("resolveUpstream(%q) = (%q, %q), want (%q, %q)", tc.path, prefix, base, tc.wantPrefix, tc.wantBase)
			}
		})
	}
}

func TestStripProxyPolicyHeaders(t *testing.T) {
	var header fasthttp.ResponseHeader
	for _, name := range proxyPolicyHeaders {
		header.Set(name, "value")
	}
	header.Set("Content-Type", "text/css")

	stripFastHTTPProxyPolicyHeaders(&header)

	for _, name := range proxyPolicyHeaders {
		if got := string(header.Peek(name)); got != "" {
			t.Fatalf("header %q = %q, want empty", name, got)
		}
	}

	if got := string(header.Peek("Content-Type")); got != "text/css" {
		t.Fatalf("Content-Type = %q, want text/css", got)
	}
}

func TestStripFastHTTPHopByHopHeaders(t *testing.T) {
	var header fasthttp.ResponseHeader
	header.Set("Connection", "keep-alive, Transfer-Encoding, X-Close-Me")
	header.Set("Transfer-Encoding", "chunked")
	header.Set("Keep-Alive", "timeout=5")
	header.Set("X-Close-Me", "value")
	header.Set("Content-Type", "text/html")

	stripFastHTTPHopByHopHeaders(&header)

	for _, name := range []string{"Connection", "Transfer-Encoding", "Keep-Alive", "X-Close-Me"} {
		if got := string(header.Peek(name)); got != "" {
			t.Fatalf("header %q = %q, want empty", name, got)
		}
	}
	if got := string(header.Peek("Content-Type")); got != "text/html" {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
}

func TestBodyForRewriteDecodesGzip(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte("payload")); err != nil {
		t.Fatalf("gzip Write() error = %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}

	var res fasthttp.Response
	res.Header.Set("Content-Encoding", "gzip")
	res.SetBody(buf.Bytes())

	body, err := bodyForRewrite(&res)
	if err != nil {
		t.Fatalf("bodyForRewrite() error = %v", err)
	}
	if string(body) != "payload" {
		t.Fatalf("bodyForRewrite() = %q, want payload", body)
	}
}

func TestBodyForRewriteRejectsUnsupportedEncoding(t *testing.T) {
	var res fasthttp.Response
	res.Header.Set("Content-Encoding", "zstd")
	res.SetBodyString("payload")

	if _, err := bodyForRewrite(&res); err == nil {
		t.Fatal("bodyForRewrite() error = nil, want error")
	}
}

func TestRewriteHTMLDocument(t *testing.T) {
	body := []byte(`
		<html>
			<head>
				<title>Steam Community :: Test</title>
				<script src="https://example.com/bad.js"></script>
				<link href="https://steamcommunity.com/public/shared.css" rel="stylesheet">
			</head>
			<body>
				<span class="valve_links"></span>
				<a onclick="SubscribeItem('123','730');"></a>
				<img src="https://images.steamusercontent.com/image.png">
			</body>
		</html>
	`)

	rewritten, err := rewriteHTMLDocument(body)
	if err != nil {
		t.Fatalf("rewriteHTMLDocument() error = %v", err)
	}

	html := string(rewritten)

	for _, want := range []string{
		"lanify-swdl :: Test",
		`href="/api/workshop/730/123"`,
		"/public/shared.css",
		"/steamusercontent-images/image.png",
		"lanify-swdl GitHub",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rewritten html missing %q:\n%s", want, html)
		}
	}

	if strings.Contains(html, "https://example.com/bad.js") {
		t.Fatalf("rewritten html still contains disallowed script source:\n%s", html)
	}
}
