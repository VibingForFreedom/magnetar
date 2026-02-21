package crawler

import "time"

// Config holds crawler pipeline configuration.
type Config struct {
	ScalingFactor                uint
	BootstrapNodes               []string
	ReseedBootstrapNodesInterval time.Duration
	SaveFilesThreshold           uint
	RescrapeThreshold            time.Duration
	Port                         uint16
}

func NewDefaultConfig() Config {
	return Config{
		ScalingFactor:                10,
		BootstrapNodes:               defaultBootstrapNodes,
		ReseedBootstrapNodesInterval: time.Minute,
		SaveFilesThreshold:           100,
		RescrapeThreshold:            time.Hour * 24 * 30,
		Port:                         6881,
	}
}

var defaultBootstrapNodes = []string{
	"router.utorrent.com:6881",
	"router.bittorrent.com:6881",
	"dht.transmissionbt.com:6881",
	"dht.aelitis.com:6881",
	"router.silotis.us:6881",
	"dht.libtorrent.org:25401",
}
