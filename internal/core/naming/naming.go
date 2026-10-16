package naming

import (
	"crypto/sha256"
	"fmt"
	"net"
	"strings"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
)

var characters = []string{"G-Man", "Titan Speaker", "Scientist", "Cameraman", "Titan Cameraman", "TV Man", "Titan TV Man", "Plungerman", "DJ Toilet", "Clockman", "Large Speakerman", "Dark Speakerman"}
var tldCountry = map[string][3]string{"de": {"DEU", "🇩🇪"}, "jp": {"JPN", "🇯🇵"}, "kr": {"KOR", "🇰🇷"}, "uk": {"GBR", "🇬🇧"}, "us": {"USA", "🇺🇸"}, "ca": {"CAN", "🇨🇦"}, "fr": {"FRA", "🇫🇷"}, "nl": {"NLD", "🇳🇱"}, "ru": {"RUS", "🇷🇺"}, "ua": {"UKR", "🇺🇦"}, "pl": {"POL", "🇵🇱"}, "cz": {"CZE", "🇨🇿"}, "fi": {"FIN", "🇫🇮"}, "se": {"SWE", "🇸🇪"}, "no": {"NOR", "🇳🇴"}, "dk": {"DNK", "🇩🇰"}, "ch": {"CHE", "🇨🇭"}, "at": {"AUT", "🇦🇹"}, "it": {"ITA", "🇮🇹"}, "es": {"ESP", "🇪🇸"}, "pt": {"PRT", "🇵🇹"}, "br": {"BRA", "🇧🇷"}, "au": {"AUS", "🇦🇺"}, "sg": {"SGP", "🇸🇬"}, "hk": {"HKG", "🇭🇰"}, "tw": {"TWN", "🇹🇼"}, "in": {"IND", "🇮🇳"}, "tr": {"TUR", "🇹🇷"}}

func Generate(node *model.NodeInfo) {
	if node == nil {
		return
	}
	raw := strings.ToLower(node.Host) + "|" + node.Port
	sum := sha256.Sum256([]byte(raw))
	node.ID = fmt.Sprintf("%x", sum[:8])
	country := strings.ToUpper(strings.TrimSpace(node.Country))
	flag := "🌐"
	if len(country) != 3 {
		country, flag = detectCountry(node.Host)
	}
	if country == "" {
		country = "UNK"
	}
	idx := int(sum[0]) % len(characters)
	suffix := "🚽"
	if sum[1]%2 == 0 {
		suffix = "📺"
	}
	node.DisplayName = fmt.Sprintf("%s_%s %s %s", flag, country, characters[idx], suffix)
}
func detectCountry(host string) (string, string) {
	host = strings.ToLower(strings.TrimSpace(host))
	if ip := net.ParseIP(host); ip != nil {
		return "UNK", "🌐"
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return "UNK", "🌐"
	}
	if c, ok := tldCountry[parts[len(parts)-1]]; ok {
		return c[0], c[1]
	}
	return "UNK", "🌐"
}

// OriginalDisplayName removes one recognized service prefix from a display name.
// The returned value is still display metadata; callers must keep NodeID as identity.
func OriginalDisplayName(displayName string) string {
	name := strings.TrimSpace(displayName)
	if i := strings.IndexByte(name, '_'); i > 0 && isKnownServicePrefix(strings.ToUpper(name[:i])) {
		return strings.TrimSpace(name[i+1:])
	}
	return name
}

// ServicePrefix returns the stable prefix for a configured service.
func ServicePrefix(serviceID string) string {
	switch strings.ToLower(strings.TrimSpace(serviceID)) {
	case "youtube", "yt":
		return "YT"
	case "telegram", "tg":
		return "TG"
	case "discord", "dc":
		return "DC"
	case "openai", "ai":
		return "AI"
	case "instagram", "ig":
		return "IG"
	default:
		return ""
	}
}

// ServiceDisplayName builds an idempotent service-scoped display name without
// changing the original node identity or URI.
func ServiceDisplayName(serviceID, originalName string) string {
	prefix := ServicePrefix(serviceID)
	name := OriginalDisplayName(originalName)
	if name == "" || prefix == "" {
		return name
	}
	return prefix + "_" + name
}

func isKnownServicePrefix(prefix string) bool {
	switch prefix {
	case "YT", "TG", "DC", "AI", "IG":
		return true
	default:
		return false
	}
}
