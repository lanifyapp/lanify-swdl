package main

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestBindTwoParams(t *testing.T) {
	var ctx fasthttp.RequestCtx

	if !bindTwoParams(&ctx, "/api/workshop/730/123", "/api/workshop/", "app_id", "workshop_id") {
		t.Fatal("bindTwoParams() = false, want true")
	}

	if got := ctx.UserValue("app_id"); got != "730" {
		t.Fatalf("app_id = %v, want 730", got)
	}

	if got := ctx.UserValue("workshop_id"); got != "123" {
		t.Fatalf("workshop_id = %v, want 123", got)
	}
}

func TestBindTwoParamsRejectsMalformedPath(t *testing.T) {
	tests := []string{
		"/api/workshop/730",
		"/api/workshop/730/123/extra",
		"/api/collection/730/123",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			var ctx fasthttp.RequestCtx
			if bindTwoParams(&ctx, path, "/api/workshop/", "app_id", "workshop_id") {
				t.Fatal("bindTwoParams() = true, want false")
			}
		})
	}
}

func TestRouteRequestRootRedirect(t *testing.T) {
	ctx := newTestRequestCtx(fasthttp.MethodGet, "/")

	routeRequest(nil)(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", got, fasthttp.StatusMovedPermanently)
	}

	if got := string(ctx.Response.Header.Peek("Location")); got != "/workshop/" {
		t.Fatalf("Location = %q, want /workshop/", got)
	}
}

func TestRouteRequestFavicon(t *testing.T) {
	ctx := newTestRequestCtx(fasthttp.MethodGet, "/favicon.svg")

	routeRequest(nil)(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("status = %d, want %d", got, fasthttp.StatusOK)
	}
	if got := string(ctx.Response.Header.ContentType()); got != "image/svg+xml" {
		t.Fatalf("Content-Type = %q, want image/svg+xml", got)
	}
	if len(ctx.Response.Body()) == 0 {
		t.Fatal("favicon response body is empty")
	}
}

func TestRouteRequestNotFound(t *testing.T) {
	ctx := newTestRequestCtx(fasthttp.MethodGet, "/not-found")

	routeRequest(nil)(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusNotFound {
		t.Fatalf("status = %d, want %d", got, fasthttp.StatusNotFound)
	}
}

func TestIsAccountAccessRoute(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/login", want: true},
		{path: "/login/home/", want: true},
		{path: "/register", want: true},
		{path: "/join/", want: true},
		{path: "/account/preferences", want: true},
		{path: "/steamcommunity/login/", want: true},
		{path: "/steamcommunity/register/", want: true},
		{path: "/steamcommunity/account/", want: true},
		{path: "/workshop/", want: false},
		{path: "/steamcommunity/actions/SearchApps/test", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := isAccountAccessRoute(tc.path); got != tc.want {
				t.Fatalf("isAccountAccessRoute(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestIsProxyRouteSupportsExactAndNestedPaths(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/workshop", want: true},
		{path: "/workshop/filedetails/", want: true},
		{path: "/steamcommunity", want: true},
		{path: "/steamcommunity/actions/SearchApps/test", want: true},
		{path: "/workshop-extra", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := isProxyRoute(tc.path); got != tc.want {
				t.Fatalf("isProxyRoute(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func newTestRequestCtx(method string, path string) *fasthttp.RequestCtx {
	var req fasthttp.Request
	req.Header.SetMethod(method)
	req.SetRequestURI(path)

	var ctx fasthttp.RequestCtx
	ctx.Init(&req, nil, nil)
	return &ctx
}
