package handler

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/lanifyapp/lanify-swdl/internal/steamcmd"
	"github.com/valyala/fasthttp"
)

var infoRegex = regexp.MustCompile(`(?i)(?:SubscribeItem|SubscribeCollection|SubscribeCollectionItem)\(\s*'(\d+)',\s*'(\d+)'\s*\);`)

const (
	projectName = "lanify-swdl"
	projectRepo = "https://github.com/lanifyapp/lanify-swdl"
)

type App struct {
	steamcmd      *steamcmd.SteamCMD
	saveDirectory string
	proxyClient   *fasthttp.Client
}

type upstreamRoute struct {
	Prefix string
	Base   string
}

type textReplacement struct {
	Old string
	New string
}

var upstreamRoutes = []upstreamRoute{
	{Prefix: "/steamcommunity", Base: "https://steamcommunity.com"},
	{Prefix: "/steamstatic-community", Base: "https://community.fastly.steamstatic.com"},
	{Prefix: "/steamstatic-cdn", Base: "https://cdn.fastly.steamstatic.com"},
	{Prefix: "/steamstatic-shared", Base: "https://shared.fastly.steamstatic.com"},
	{Prefix: "/steamstatic-clan", Base: "https://clan.fastly.steamstatic.com"},
	{Prefix: "/steamstatic-avatars", Base: "https://avatars.fastly.steamstatic.com"},
	{Prefix: "/steamstatic-cloudflare", Base: "https://cdn.cloudflare.steamstatic.com"},
	{Prefix: "/steamstatic-cloudflare-alt", Base: "https://steamcdn.cloudflare.steamstatic.com"},
	{Prefix: "/steamstore", Base: "https://store.steampowered.com"},
	{Prefix: "/steamcdn-akamai", Base: "https://steamcdn-a.akamaihd.net"},
	{Prefix: "/steam-media", Base: "https://media.steampowered.com"},
	{Prefix: "/steam-video", Base: "https://video.fastly.steamstatic.com"},
	{Prefix: "/steamusercontent-images", Base: "https://images.steamusercontent.com"},
	{Prefix: "/steamusercontent", Base: "https://steamusercontent-a.akamaihd.net"},
}

var textReplacements = []textReplacement{
	{Old: "https://community.fastly.steamstatic.com", New: "/steamstatic-community"},
	{Old: "http://community.fastly.steamstatic.com", New: "/steamstatic-community"},
	{Old: "//community.fastly.steamstatic.com", New: "/steamstatic-community"},
	{Old: "https:\\/\\/community.fastly.steamstatic.com", New: "\\/steamstatic-community"},
	{Old: "http:\\/\\/community.fastly.steamstatic.com", New: "\\/steamstatic-community"},
	{Old: "\\/\\/community.fastly.steamstatic.com", New: "\\/steamstatic-community"},

	{Old: "https://community.akamai.steamstatic.com", New: "/steamstatic-community"},
	{Old: "http://community.akamai.steamstatic.com", New: "/steamstatic-community"},
	{Old: "//community.akamai.steamstatic.com", New: "/steamstatic-community"},
	{Old: "https:\\/\\/community.akamai.steamstatic.com", New: "\\/steamstatic-community"},
	{Old: "http:\\/\\/community.akamai.steamstatic.com", New: "\\/steamstatic-community"},
	{Old: "\\/\\/community.akamai.steamstatic.com", New: "\\/steamstatic-community"},

	{Old: "https://cdn.fastly.steamstatic.com", New: "/steamstatic-cdn"},
	{Old: "http://cdn.fastly.steamstatic.com", New: "/steamstatic-cdn"},
	{Old: "//cdn.fastly.steamstatic.com", New: "/steamstatic-cdn"},
	{Old: "https:\\/\\/cdn.fastly.steamstatic.com", New: "\\/steamstatic-cdn"},
	{Old: "http:\\/\\/cdn.fastly.steamstatic.com", New: "\\/steamstatic-cdn"},
	{Old: "\\/\\/cdn.fastly.steamstatic.com", New: "\\/steamstatic-cdn"},

	{Old: "https://shared.fastly.steamstatic.com", New: "/steamstatic-shared"},
	{Old: "http://shared.fastly.steamstatic.com", New: "/steamstatic-shared"},
	{Old: "//shared.fastly.steamstatic.com", New: "/steamstatic-shared"},
	{Old: "https:\\/\\/shared.fastly.steamstatic.com", New: "\\/steamstatic-shared"},
	{Old: "http:\\/\\/shared.fastly.steamstatic.com", New: "\\/steamstatic-shared"},
	{Old: "\\/\\/shared.fastly.steamstatic.com", New: "\\/steamstatic-shared"},

	{Old: "https://avatars.fastly.steamstatic.com", New: "/steamstatic-avatars"},
	{Old: "http://avatars.fastly.steamstatic.com", New: "/steamstatic-avatars"},
	{Old: "//avatars.fastly.steamstatic.com", New: "/steamstatic-avatars"},
	{Old: "https:\\/\\/avatars.fastly.steamstatic.com", New: "\\/steamstatic-avatars"},
	{Old: "http:\\/\\/avatars.fastly.steamstatic.com", New: "\\/steamstatic-avatars"},
	{Old: "\\/\\/avatars.fastly.steamstatic.com", New: "\\/steamstatic-avatars"},

	{Old: "https://clan.fastly.steamstatic.com", New: "/steamstatic-clan"},
	{Old: "http://clan.fastly.steamstatic.com", New: "/steamstatic-clan"},
	{Old: "//clan.fastly.steamstatic.com", New: "/steamstatic-clan"},
	{Old: "https:\\/\\/clan.fastly.steamstatic.com", New: "\\/steamstatic-clan"},
	{Old: "http:\\/\\/clan.fastly.steamstatic.com", New: "\\/steamstatic-clan"},
	{Old: "\\/\\/clan.fastly.steamstatic.com", New: "\\/steamstatic-clan"},

	{Old: "https://cdn.cloudflare.steamstatic.com", New: "/steamstatic-cloudflare"},
	{Old: "http://cdn.cloudflare.steamstatic.com", New: "/steamstatic-cloudflare"},
	{Old: "//cdn.cloudflare.steamstatic.com", New: "/steamstatic-cloudflare"},
	{Old: "https:\\/\\/cdn.cloudflare.steamstatic.com", New: "\\/steamstatic-cloudflare"},
	{Old: "http:\\/\\/cdn.cloudflare.steamstatic.com", New: "\\/steamstatic-cloudflare"},
	{Old: "\\/\\/cdn.cloudflare.steamstatic.com", New: "\\/steamstatic-cloudflare"},

	{Old: "https://steamcdn.cloudflare.steamstatic.com", New: "/steamstatic-cloudflare-alt"},
	{Old: "http://steamcdn.cloudflare.steamstatic.com", New: "/steamstatic-cloudflare-alt"},
	{Old: "//steamcdn.cloudflare.steamstatic.com", New: "/steamstatic-cloudflare-alt"},
	{Old: "https:\\/\\/steamcdn.cloudflare.steamstatic.com", New: "\\/steamstatic-cloudflare-alt"},
	{Old: "http:\\/\\/steamcdn.cloudflare.steamstatic.com", New: "\\/steamstatic-cloudflare-alt"},
	{Old: "\\/\\/steamcdn.cloudflare.steamstatic.com", New: "\\/steamstatic-cloudflare-alt"},

	{Old: "https://store.steampowered.com", New: "/steamstore"},
	{Old: "http://store.steampowered.com", New: "/steamstore"},
	{Old: "//store.steampowered.com", New: "/steamstore"},
	{Old: "https:\\/\\/store.steampowered.com", New: "\\/steamstore"},
	{Old: "http:\\/\\/store.steampowered.com", New: "\\/steamstore"},
	{Old: "\\/\\/store.steampowered.com", New: "\\/steamstore"},

	{Old: "https://steamcdn-a.akamaihd.net", New: "/steamcdn-akamai"},
	{Old: "http://steamcdn-a.akamaihd.net", New: "/steamcdn-akamai"},
	{Old: "//steamcdn-a.akamaihd.net", New: "/steamcdn-akamai"},
	{Old: "https:\\/\\/steamcdn-a.akamaihd.net", New: "\\/steamcdn-akamai"},
	{Old: "http:\\/\\/steamcdn-a.akamaihd.net", New: "\\/steamcdn-akamai"},
	{Old: "\\/\\/steamcdn-a.akamaihd.net", New: "\\/steamcdn-akamai"},

	{Old: "https://media.steampowered.com", New: "/steam-media"},
	{Old: "http://media.steampowered.com", New: "/steam-media"},
	{Old: "//media.steampowered.com", New: "/steam-media"},
	{Old: "https:\\/\\/media.steampowered.com", New: "\\/steam-media"},
	{Old: "http:\\/\\/media.steampowered.com", New: "\\/steam-media"},
	{Old: "\\/\\/media.steampowered.com", New: "\\/steam-media"},

	{Old: "https://video.fastly.steamstatic.com", New: "/steam-video"},
	{Old: "http://video.fastly.steamstatic.com", New: "/steam-video"},
	{Old: "//video.fastly.steamstatic.com", New: "/steam-video"},
	{Old: "https:\\/\\/video.fastly.steamstatic.com", New: "\\/steam-video"},
	{Old: "http:\\/\\/video.fastly.steamstatic.com", New: "\\/steam-video"},
	{Old: "\\/\\/video.fastly.steamstatic.com", New: "\\/steam-video"},

	{Old: "https://images.steamusercontent.com", New: "/steamusercontent-images"},
	{Old: "http://images.steamusercontent.com", New: "/steamusercontent-images"},
	{Old: "//images.steamusercontent.com", New: "/steamusercontent-images"},
	{Old: "https:\\/\\/images.steamusercontent.com", New: "\\/steamusercontent-images"},
	{Old: "http:\\/\\/images.steamusercontent.com", New: "\\/steamusercontent-images"},
	{Old: "\\/\\/images.steamusercontent.com", New: "\\/steamusercontent-images"},

	{Old: "https://steamusercontent-a.akamaihd.net", New: "/steamusercontent"},
	{Old: "http://steamusercontent-a.akamaihd.net", New: "/steamusercontent"},
	{Old: "//steamusercontent-a.akamaihd.net", New: "/steamusercontent"},
	{Old: "https:\\/\\/steamusercontent-a.akamaihd.net", New: "\\/steamusercontent"},
	{Old: "http:\\/\\/steamusercontent-a.akamaihd.net", New: "\\/steamusercontent"},
	{Old: "\\/\\/steamusercontent-a.akamaihd.net", New: "\\/steamusercontent"},

	{Old: "https://steamcommunity.com", New: "/steamcommunity"},
	{Old: "http://steamcommunity.com", New: "/steamcommunity"},
	{Old: "//steamcommunity.com", New: "/steamcommunity"},
	{Old: "https:\\/\\/steamcommunity.com", New: "\\/steamcommunity"},
	{Old: "http:\\/\\/steamcommunity.com", New: "\\/steamcommunity"},
	{Old: "\\/\\/steamcommunity.com", New: "\\/steamcommunity"},
}

var proxyPolicyHeaders = []string{
	"Content-Security-Policy",
	"Content-Security-Policy-Report-Only",
	"X-Frame-Options",
	"Cross-Origin-Resource-Policy",
	"Cross-Origin-Embedder-Policy",
	"Cross-Origin-Opener-Policy",
	"Access-Control-Allow-Origin",
	"Access-Control-Allow-Headers",
	"Access-Control-Allow-Methods",
}

var proxyHopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func New(s *steamcmd.SteamCMD) (*App, error) {
	saveDirectory := filepath.Join(s.InstallPath, "downloads")
	if err := os.MkdirAll(saveDirectory, 0755); err != nil {
		return nil, fmt.Errorf("create download directory: %w", err)
	}

	absSaveDirectory, err := filepath.Abs(saveDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve download directory: %w", err)
	}

	return &App{
		steamcmd:      s,
		saveDirectory: absSaveDirectory,
		proxyClient: &fasthttp.Client{
			ReadTimeout:                   45 * time.Second,
			WriteTimeout:                  45 * time.Second,
			MaxIdleConnDuration:           30 * time.Second,
			NoDefaultUserAgentHeader:      true,
			DisableHeaderNamesNormalizing: true,
		},
	}, nil
}

func (h *App) UnsupportedPageHandler(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(fasthttp.StatusForbidden)
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyString("Steam login and registration pages are disabled in lanify-swdl. Start the server with -steamuser and -steampassword when account access is required. <a href='/'>Back</a>")
}

func resolveUpstream(path string) (prefix string, base string, ok bool) {
	switch {
	case strings.HasPrefix(path, "/workshop/"),
		strings.HasPrefix(path, "/app/"),
		strings.HasPrefix(path, "/public/"),
		strings.HasPrefix(path, "/sharedfiles/"):
		return "", "https://steamcommunity.com", true
	default:
		for _, route := range upstreamRoutes {
			if strings.HasPrefix(path, route.Prefix+"/") || path == route.Prefix {
				return route.Prefix, route.Base, true
			}
		}

		return "", "", false
	}
}

func isAllowedExternalHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	switch host {
	case "",
		"steamcommunity.com",
		"community.fastly.steamstatic.com",
		"community.akamai.steamstatic.com",
		"cdn.fastly.steamstatic.com",
		"shared.fastly.steamstatic.com",
		"avatars.fastly.steamstatic.com",
		"clan.fastly.steamstatic.com",
		"cdn.cloudflare.steamstatic.com",
		"steamcdn.cloudflare.steamstatic.com",
		"store.steampowered.com",
		"steamcdn-a.akamaihd.net",
		"media.steampowered.com",
		"video.fastly.steamstatic.com",
		"images.steamusercontent.com",
		"steamusercontent-a.akamaihd.net":
		return true
	default:
		return false
	}
}

func parseExternalHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return strings.ToLower(u.Host)
}

func rewriteSteamCommunityURI(requestURI string) string {
	switch {
	case strings.HasPrefix(requestURI, "/workshop/"),
		strings.HasPrefix(requestURI, "/app/"),
		strings.HasPrefix(requestURI, "/public/"),
		strings.HasPrefix(requestURI, "/sharedfiles/"):
		return requestURI
	default:
		return "/steamcommunity" + requestURI
	}
}

func rewriteProxiedURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}

	if strings.HasPrefix(raw, "#") ||
		strings.HasPrefix(raw, "mailto:") ||
		strings.HasPrefix(raw, "javascript:") ||
		strings.HasPrefix(raw, "data:") ||
		strings.HasPrefix(raw, "blob:") {
		return raw
	}

	wrappedTopLocation := false
	if strings.HasPrefix(raw, "top.location.href='") && strings.HasSuffix(raw, "'") {
		raw = strings.TrimPrefix(raw, "top.location.href='")
		raw = strings.TrimSuffix(raw, "'")
		wrappedTopLocation = true
	}

	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		if wrappedTopLocation {
			return "top.location.href='" + raw + "'"
		}
		return raw
	}

	if u.Host == "" {
		if wrappedTopLocation {
			return "top.location.href='" + raw + "'"
		}
		return raw
	}

	host := strings.ToLower(u.Host)
	if !isAllowedExternalHost(host) {
		if wrappedTopLocation {
			return "top.location.href='" + raw + "'"
		}
		return raw
	}

	var rewritten string

	switch host {
	case "steamcommunity.com":
		rewritten = rewriteSteamCommunityURI(u.RequestURI())
	case "community.fastly.steamstatic.com", "community.akamai.steamstatic.com":
		rewritten = "/steamstatic-community" + u.RequestURI()
	case "cdn.fastly.steamstatic.com":
		rewritten = "/steamstatic-cdn" + u.RequestURI()
	case "shared.fastly.steamstatic.com":
		rewritten = "/steamstatic-shared" + u.RequestURI()
	case "avatars.fastly.steamstatic.com":
		rewritten = "/steamstatic-avatars" + u.RequestURI()
	case "clan.fastly.steamstatic.com":
		rewritten = "/steamstatic-clan" + u.RequestURI()
	case "cdn.cloudflare.steamstatic.com":
		rewritten = "/steamstatic-cloudflare" + u.RequestURI()
	case "steamcdn.cloudflare.steamstatic.com":
		rewritten = "/steamstatic-cloudflare-alt" + u.RequestURI()
	case "store.steampowered.com":
		rewritten = "/steamstore" + u.RequestURI()
	case "steamcdn-a.akamaihd.net":
		rewritten = "/steamcdn-akamai" + u.RequestURI()
	case "media.steampowered.com":
		rewritten = "/steam-media" + u.RequestURI()
	case "video.fastly.steamstatic.com":
		rewritten = "/steam-video" + u.RequestURI()
	case "images.steamusercontent.com":
		rewritten = "/steamusercontent-images" + u.RequestURI()
	case "steamusercontent-a.akamaihd.net":
		rewritten = "/steamusercontent" + u.RequestURI()
	default:
		if wrappedTopLocation {
			return "top.location.href='" + raw + "'"
		}
		return raw
	}

	if wrappedTopLocation {
		return "top.location.href='" + rewritten + "'"
	}

	return rewritten
}

func rewriteTextContent(body []byte) []byte {
	out := body
	for _, replacement := range textReplacements {
		out = bytes.ReplaceAll(out, []byte(replacement.Old), []byte(replacement.New))
	}

	for _, firstClassRoute := range []string{"workshop", "app", "public", "sharedfiles"} {
		out = bytes.ReplaceAll(out, []byte("/steamcommunity/"+firstClassRoute+"/"), []byte("/"+firstClassRoute+"/"))
		out = bytes.ReplaceAll(out, []byte("\\/steamcommunity\\/"+firstClassRoute+"\\/"), []byte("\\/"+firstClassRoute+"\\/"))
	}

	return out
}

func isRewriteableTextContent(contentType string) bool {
	contentType = strings.ToLower(contentType)

	return strings.Contains(contentType, "text/") ||
		strings.Contains(contentType, "javascript") ||
		strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "xml") ||
		strings.Contains(contentType, "svg") ||
		strings.Contains(contentType, "css")
}

func rewriteHTMLDocument(body []byte) ([]byte, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	titleElm := doc.Find("title").First()
	if titleElm.Length() > 0 {
		title := titleElm.Text()
		title = strings.Replace(title, "Steam Community", projectName, 1)
		titleElm.SetText(title)
	}

	replacedCollectionPrimary := false

	doc.Find("a[onclick*='SubscribeItem']").Each(func(_ int, s *goquery.Selection) {
		onclick, _ := s.Attr("onclick")
		matches := infoRegex.FindStringSubmatch(onclick)
		if len(matches) != 3 {
			return
		}

		workshopID := matches[1]
		appID := matches[2]

		button := fmt.Sprintf(
			`<div><a href="/api/workshop/%s/%s" class="btn_darkred_white_innerfade btn_border_2px btn_medium" style="position: relative"><div class="followIcon"></div><span class="subscribeText"><div>Download</div></span></a></div>`,
			appID,
			workshopID,
		)

		s.Parent().Parent().AppendHtml(button)
	})

	doc.Find(`span[class="valve_links"]`).Each(func(_ int, s *goquery.Selection) {
		link := fmt.Sprintf(` | <a style="color: #1497cb; font-weight: bold; font-size: medium;" href="%s" target="_blank">%s GitHub</a>`, projectRepo, projectName)
		s.AppendHtml(link)
	})

	doc.Find(".subscribe[onclick*='SubscribeCollection']").Each(func(_ int, s *goquery.Selection) {
		onclick, _ := s.Attr("onclick")
		matches := infoRegex.FindStringSubmatch(onclick)
		if len(matches) != 3 {
			return
		}

		collectionID := matches[1]
		appID := matches[2]

		var button string

		if !replacedCollectionPrimary {
			button = fmt.Sprintf(
				`<a class="general_btn subscribe" style="background: #640000; color: white; display: table;" href="/api/collection/%s/%s"><div class="followIcon"></div><span class="subscribeText">Download Collection</span></a>`,
				appID,
				collectionID,
			)
			replacedCollectionPrimary = true
		} else {
			button = fmt.Sprintf(
				`<div><a href="/api/workshop/%s/%s" class="btn_darkred_white_innerfade btn_medium" style="position: relative"><div class="followIcon"></div><span class="subscribeText"><div>Download</div></span></a></div>`,
				appID,
				collectionID,
			)
		}

		s.Parent().AppendHtml(button)
	})

	doc.Find("script[src]").Each(func(_ int, s *goquery.Selection) {
		val, exists := s.Attr("src")
		if !exists {
			return
		}

		host := parseExternalHost(val)
		if host != "" && !isAllowedExternalHost(host) {
			s.Remove()
			return
		}

		s.SetAttr("src", rewriteProxiedURL(val))
	})

	doc.Find(`link[href]`).Each(func(_ int, s *goquery.Selection) {
		val, exists := s.Attr("href")
		if !exists {
			return
		}

		host := parseExternalHost(val)
		if host != "" && !isAllowedExternalHost(host) {
			s.Remove()
			return
		}

		s.SetAttr("href", rewriteProxiedURL(val))
	})

	doc.Find("img[src]").Each(func(_ int, s *goquery.Selection) {
		val, exists := s.Attr("src")
		if !exists {
			return
		}

		host := parseExternalHost(val)
		if host != "" && !isAllowedExternalHost(host) {
			s.RemoveAttr("src")
			return
		}

		s.SetAttr("src", rewriteProxiedURL(val))
	})

	doc.Find("iframe[src]").Each(func(_ int, s *goquery.Selection) {
		val, exists := s.Attr("src")
		if !exists {
			return
		}

		host := parseExternalHost(val)
		if host != "" && !isAllowedExternalHost(host) {
			s.Remove()
			return
		}

		s.SetAttr("src", rewriteProxiedURL(val))
	})

	doc.Find("[href]").Each(func(_ int, s *goquery.Selection) {
		if val, exists := s.Attr("href"); exists {
			s.SetAttr("href", rewriteProxiedURL(val))
		}
	})

	doc.Find("[src]").Each(func(_ int, s *goquery.Selection) {
		if val, exists := s.Attr("src"); exists {
			s.SetAttr("src", rewriteProxiedURL(val))
		}
	})

	doc.Find("[action]").Each(func(_ int, s *goquery.Selection) {
		if val, exists := s.Attr("action"); exists {
			s.SetAttr("action", rewriteProxiedURL(val))
		}
	})

	doc.Find("[onclick]").Each(func(_ int, s *goquery.Selection) {
		if val, exists := s.Attr("onclick"); exists {
			s.SetAttr("onclick", rewriteProxiedURL(val))
		}
	})

	doc.Find("[srcset]").Each(func(_ int, s *goquery.Selection) {
		val, exists := s.Attr("srcset")
		if !exists {
			return
		}

		var rewritten []string

		for _, part := range strings.Split(val, ",") {
			part = strings.TrimSpace(part)
			fields := strings.Fields(part)
			if len(fields) == 0 {
				continue
			}

			fields[0] = rewriteProxiedURL(fields[0])
			rewritten = append(rewritten, strings.Join(fields, " "))
		}

		s.SetAttr("srcset", strings.Join(rewritten, ", "))
	})

	html, err := doc.Html()
	if err != nil {
		return nil, err
	}

	return rewriteTextContent([]byte(html)), nil
}

func stripFastHTTPProxyPolicyHeaders(header *fasthttp.ResponseHeader) {
	for _, name := range proxyPolicyHeaders {
		header.Del(name)
	}
}

func stripFastHTTPHopByHopHeaders(header *fasthttp.ResponseHeader) {
	if connection := string(header.Peek("Connection")); connection != "" {
		for _, name := range strings.Split(connection, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				header.Del(name)
			}
		}
	}

	for _, name := range proxyHopByHopHeaders {
		header.Del(name)
	}
}

func (h *App) SteamProxyHandler(ctx *fasthttp.RequestCtx) {
	requestPath := string(ctx.Path())
	prefix, upstreamBase, ok := resolveUpstream(requestPath)
	if !ok {
		ctx.Error("Unsupported proxied path", fasthttp.StatusBadGateway)
		return
	}

	remote, err := url.Parse(upstreamBase)
	if err != nil {
		ctx.Error("Failed to parse upstream", fasthttp.StatusInternalServerError)
		return
	}

	upstreamPath := requestPath
	if prefix != "" && strings.HasPrefix(upstreamPath, prefix) {
		upstreamPath = strings.TrimPrefix(upstreamPath, prefix)
	}
	if upstreamPath == "" {
		upstreamPath = "/"
	}

	upstreamURL := remote.Scheme + "://" + remote.Host + upstreamPath
	if query := string(ctx.QueryArgs().QueryString()); query != "" {
		upstreamURL += "?" + query
	}

	req := fasthttp.AcquireRequest()
	res := fasthttp.AcquireResponse()

	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(res)

	req.SetRequestURI(upstreamURL)
	req.Header.SetMethodBytes(ctx.Method())
	req.Header.SetHost(remote.Host)
	req.SetBodyRaw(ctx.PostBody())

	order := ctx.Request.Header.AllInOrder()

	order(func(key []byte, value []byte) bool {
		req.Header.SetBytesKV(key, value)
		return true
	})

	req.Header.SetHost(remote.Host)
	req.Header.Set("Accept-Encoding", "identity")

	if err = h.proxyClient.Do(req, res); err != nil {
		ctx.Error(fmt.Sprintf("Proxy request failed: %v", err), fasthttp.StatusBadGateway)
		return
	}

	res.CopyTo(&ctx.Response)
	stripFastHTTPHopByHopHeaders(&ctx.Response.Header)

	if location := res.Header.Peek("Location"); len(location) > 0 {
		ctx.Response.Header.Set("Location", rewriteProxiedURL(string(location)))
	}

	contentType := strings.ToLower(string(res.Header.Peek("Content-Type")))

	if !strings.Contains(contentType, "text/html") && !isRewriteableTextContent(contentType) {
		stripFastHTTPProxyPolicyHeaders(&ctx.Response.Header)
		return
	}

	body, err := bodyForRewrite(res)
	if err != nil {
		ctx.Error(fmt.Sprintf("Failed to decode upstream body: %v", err), fasthttp.StatusBadGateway)
		return
	}

	switch {
	case strings.Contains(contentType, "text/html"):
		rewritten, err := rewriteHTMLDocument(body)
		if err != nil {
			ctx.Error(fmt.Sprintf("Failed to rewrite HTML: %v", err), fasthttp.StatusInternalServerError)
			return
		}

		body = rewritten
	case isRewriteableTextContent(contentType):
		body = rewriteTextContent(body)
	}

	ctx.SetBody(body)
	ctx.Response.Header.Del("Content-Encoding")
	stripFastHTTPProxyPolicyHeaders(&ctx.Response.Header)
}

func bodyForRewrite(res *fasthttp.Response) ([]byte, error) {
	encoding := strings.ToLower(strings.TrimSpace(string(res.Header.Peek("Content-Encoding"))))

	switch encoding {
	case "", "identity":
		return res.Body(), nil
	case "gzip", "x-gzip":
		return res.BodyGunzip()
	case "br":
		return res.BodyUnbrotli()
	case "deflate":
		return res.BodyInflate()
	default:
		return nil, fmt.Errorf("unsupported content encoding %q", encoding)
	}
}
