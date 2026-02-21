package tracker

import (
	"fmt"
	"net/url"
	"strings"
)

type trackerProto int

const (
	protoUDP trackerProto = iota
	protoHTTP
)

type trackerURL struct {
	proto    trackerProto
	host     string // host:port
	scrapeURL string // full scrape URL for HTTP trackers
}

// parseTrackerURL parses a tracker announce URL into a structured form.
func parseTrackerURL(rawURL string) (trackerURL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return trackerURL{}, fmt.Errorf("parsing tracker URL: %w", err)
	}

	switch u.Scheme {
	case "udp":
		host := u.Host
		if !strings.Contains(host, ":") {
			host += ":80"
		}
		return trackerURL{proto: protoUDP, host: host}, nil

	case "http", "https":
		scrape := announcToScrape(rawURL)
		if scrape == "" {
			return trackerURL{}, fmt.Errorf("cannot derive scrape URL from %q", rawURL)
		}
		return trackerURL{proto: protoHTTP, host: u.Host, scrapeURL: scrape}, nil

	default:
		return trackerURL{}, fmt.Errorf("unsupported tracker scheme: %q", u.Scheme)
	}
}

// announcToScrape converts an announce URL to its scrape equivalent.
// Returns empty string if conversion is not possible.
func announcToScrape(announceURL string) string {
	idx := strings.LastIndex(announceURL, "/announce")
	if idx == -1 {
		return ""
	}
	return announceURL[:idx] + "/scrape" + announceURL[idx+len("/announce"):]
}
