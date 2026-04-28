package steam

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetWorkshopNameRetriesAfter429(t *testing.T) {
	originalBaseURL := steamCommunityBaseURL
	originalSleep := steamHTTPSleep

	t.Cleanup(func() {
		steamCommunityBaseURL = originalBaseURL
		steamHTTPSleep = originalSleep
	})

	var sleepCalls []time.Duration

	steamHTTPSleep = func(d time.Duration) {
		sleepCalls = append(sleepCalls, d)
	}

	requests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<div class="workshopItemTitle">Test Item</div>`))
	}))

	defer server.Close()

	steamCommunityBaseURL = server.URL

	name, err := GetWorkshopName(123)
	if err != nil {
		t.Fatalf("GetWorkshopName() error = %v", err)
	}

	if name != "Test Item" {
		t.Fatalf("GetWorkshopName() name = %q, want %q", name, "Test Item")
	}

	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}

	if len(sleepCalls) != 1 || sleepCalls[0] != 2*time.Second {
		t.Fatalf("sleep calls = %v, want [2s]", sleepCalls)
	}
}

func TestGetCollectionItemsFailsAfterMax429Retries(t *testing.T) {
	originalBaseURL := steamCommunityBaseURL
	originalSleep := steamHTTPSleep

	t.Cleanup(func() {
		steamCommunityBaseURL = originalBaseURL
		steamHTTPSleep = originalSleep
	})

	sleepCount := 0
	steamHTTPSleep = func(time.Duration) {
		sleepCount++
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))

	defer server.Close()

	steamCommunityBaseURL = server.URL

	_, _, err := GetCollectionItems(456)
	if err == nil {
		t.Fatal("GetCollectionItems() error = nil, want error")
	}

	if sleepCount != maxSteamRetryAttempts-1 {
		t.Fatalf("sleep count = %d, want %d", sleepCount, maxSteamRetryAttempts-1)
	}
}

func TestGetCollectionItemsParsesUniqueItemsAndFallbackTitles(t *testing.T) {
	originalBaseURL := steamCommunityBaseURL

	t.Cleanup(func() {
		steamCommunityBaseURL = originalBaseURL
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.TrimSpace(`
			<div class="workshopItemTitle">Example Collection</div>
			<div class="collectionItem">
				<a href="https://steamcommunity.com/sharedfiles/filedetails/?id=111"></a>
				<div class="collectionItemTitle">First Item</div>
			</div>
			<div class="collectionItem">
				<a href="https://steamcommunity.com/sharedfiles/filedetails/?id=111"></a>
				<div class="collectionItemTitle">Duplicate Item</div>
			</div>
			<div class="collectionItem">
				<a href="https://steamcommunity.com/sharedfiles/filedetails/?id=222"></a>
				<img alt="Alt Title" />
			</div>
		`)))
	}))

	defer server.Close()

	steamCommunityBaseURL = server.URL

	title, items, err := GetCollectionItems(999)
	if err != nil {
		t.Fatalf("GetCollectionItems() error = %v", err)
	}

	if title != "Example Collection" {
		t.Fatalf("title = %q, want %q", title, "Example Collection")
	}

	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	if items[0].ID != 111 || items[0].Title != "First Item" {
		t.Fatalf("items[0] = %+v, want ID=111 Title=%q", items[0], "First Item")
	}

	if items[1].ID != 222 || items[1].Title != "Alt Title" {
		t.Fatalf("items[1] = %+v, want ID=222 Title=%q", items[1], "Alt Title")
	}
}
