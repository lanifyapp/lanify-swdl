package steam

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	
	"github.com/PuerkitoBio/goquery"
)

var steamHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

var steamCommunityBaseURL = "https://steamcommunity.com"

var steamHTTPSleep = time.Sleep

const maxSteamRetryAttempts = 3

type WorkshopItem struct {
	ID    int
	Title string
}

func steamWorkshopDetailsURL(id int) string {
	return fmt.Sprintf(
		"%s/sharedfiles/filedetails/?id=%d",
		strings.TrimRight(steamCommunityBaseURL, "/"),
		id,
	)
}

func parseRetryAfter(header string) (time.Duration, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, false
	}
	
	if seconds, err := strconv.Atoi(header); err == nil {
		if seconds < 0 {
			seconds = 0
		}
		
		return time.Duration(seconds) * time.Second, true
	}
	
	if when, err := http.ParseTime(header); err == nil {
		wait := time.Until(when)
		if wait < 0 {
			wait = 0
		}
		
		return wait, true
	}
	
	return 0, false
}

func retryDelay(attempt int, retryAfter string) time.Duration {
	if wait, ok := parseRetryAfter(retryAfter); ok {
		return wait
	}
	
	if attempt < 1 {
		attempt = 1
	}
	
	return time.Duration(attempt) * time.Second
}

func getSteamPage(pageURL string) (*http.Response, error) {
	var lastErr error
	
	for attempt := 1; attempt <= maxSteamRetryAttempts; attempt++ {
		res, err := steamHTTPClient.Get(pageURL)
		if err != nil {
			return nil, err
		}
		
		if res.StatusCode != http.StatusTooManyRequests {
			return res, nil
		}
		
		lastErr = fmt.Errorf("steam returned status %d", res.StatusCode)
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		
		if attempt == maxSteamRetryAttempts {
			break
		}
		
		steamHTTPSleep(retryDelay(attempt, res.Header.Get("Retry-After")))
	}
	
	return nil, lastErr
}

func GetWorkshopName(workshopID int) (string, error) {
	pageURL := steamWorkshopDetailsURL(workshopID)
	
	res, err := getSteamPage(pageURL)
	if err != nil {
		return "", err
	}
	
	defer res.Body.Close()
	
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("steam returned status %d", res.StatusCode)
	}
	
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", err
	}
	
	title := strings.TrimSpace(doc.Find("div.workshopItemTitle").First().Text())
	if title == "" {
		return "", fmt.Errorf("could not find title for workshop item %d", workshopID)
	}
	
	return title, nil
}

func extractWorkshopIDFromURL(raw string) (int, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	
	idStr := strings.TrimSpace(u.Query().Get("id"))
	if idStr == "" {
		return 0, fmt.Errorf("missing id query parameter")
	}
	
	return strconv.Atoi(idStr)
}

func GetCollectionItems(collectionID int) (string, []WorkshopItem, error) {
	pageURL := steamWorkshopDetailsURL(collectionID)
	
	res, err := getSteamPage(pageURL)
	if err != nil {
		return "", nil, err
	}
	
	defer res.Body.Close()
	
	if res.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("steam returned status %d", res.StatusCode)
	}
	
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", nil, err
	}
	
	collectionTitle := strings.TrimSpace(doc.Find("div.workshopItemTitle").First().Text())
	
	var items []WorkshopItem
	
	seen := make(map[int]struct{})
	
	doc.Find("div.collectionItem").Each(func(_ int, s *goquery.Selection) {
		var rawHref string
		
		s.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			href, exists := a.Attr("href")
			if !exists {
				return true
			}
			
			if strings.Contains(href, "filedetails/?id=") {
				rawHref = href
				return false
			}
			
			return true
		})
		
		if rawHref == "" {
			return
		}
		
		id, err := extractWorkshopIDFromURL(rawHref)
		if err != nil {
			return
		}
		
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		
		title := strings.TrimSpace(s.Find("div.collectionItemTitle").First().Text())
		if title == "" {
			title = strings.TrimSpace(s.Find(".workshopItemTitle").First().Text())
		}
		
		if title == "" {
			title = strings.TrimSpace(s.Find("img[alt]").First().AttrOr("alt", ""))
		}
		
		items = append(items, WorkshopItem{
			ID:    id,
			Title: title,
		})
	})
	
	return collectionTitle, items, nil
}
