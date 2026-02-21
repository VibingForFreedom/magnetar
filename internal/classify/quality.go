package classify

import (
	"strings"
)

type Quality int

const (
	QualityUnknown Quality = iota
	QualitySD
	QualityHD
	QualityFHD
	QualityUHD
)

func (q Quality) String() string {
	switch q {
	case QualitySD:
		return "SD"
	case QualityHD:
		return "HD"
	case QualityFHD:
		return "FHD"
	case QualityUHD:
		return "UHD"
	default:
		return "Unknown"
	}
}

func DetectQuality(name string) Quality {
	n := strings.ToLower(name)

	if strings.Contains(n, "2160p") || strings.Contains(n, "4k") || strings.Contains(n, "uhd") {
		if strings.Contains(n, "4kuhd") || strings.Contains(n, "2160p") {
			return QualityUHD
		}
		if strings.Contains(n, "uhd") {
			return QualityUHD
		}
		return QualityUHD
	}

	if strings.Contains(n, "1080p") || strings.Contains(n, "1080i") || strings.Contains(n, "fhd") || strings.Contains(n, "fullhd") {
		return QualityFHD
	}

	if strings.Contains(n, "720p") || strings.Contains(n, "720i") {
		return QualityHD
	}

	if strings.Contains(n, "480p") || strings.Contains(n, "480i") ||
		strings.Contains(n, "576p") || strings.Contains(n, "576i") ||
		strings.Contains(n, "dvdrip") || strings.Contains(n, "dvd-rip") ||
		strings.Contains(n, "sdtv") || strings.Contains(n, "dsr") ||
		strings.Contains(n, "xvid") || strings.Contains(n, "divx") {
		return QualitySD
	}

	return QualityUnknown
}

func DetectQualityFromResolution(res string) Quality {
	switch strings.ToLower(res) {
	case "480p", "480i", "576p", "576i":
		return QualitySD
	case "720p", "720i":
		return QualityHD
	case "1080p", "1080i":
		return QualityFHD
	case "2160p", "4k", "uhd":
		return QualityUHD
	default:
		return QualityUnknown
	}
}
