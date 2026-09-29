package scraper

import "strings"

// Lang is a UI language preference, decoupled from the per-provider codes that
// have to be sent on the wire.
type Lang string

const (
	LangChinese  Lang = "zh-CN"
	LangEnglish  Lang = "en-US"
	LangJapanese Lang = "ja-JP"
)

// ParseLang normalises a configured language string.
func ParseLang(s string) Lang {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "zh-cn", "zh", "schinese", "zh-hans":
		return LangChinese
	case "ja-jp", "ja", "japanese":
		return LangJapanese
	default:
		return LangEnglish
	}
}

// SteamCode is the value Steam's appdetails API expects for "l".
func (l Lang) SteamCode() string {
	switch l {
	case LangChinese:
		return "schinese"
	case LangJapanese:
		return "japanese"
	default:
		return "english"
	}
}

// VNDBCode is the value the VNDB Kana API expects for a language filter.
func (l Lang) VNDBCode() string {
	switch l {
	case LangChinese:
		return "zh-Hans"
	case LangJapanese:
		return "ja"
	default:
		return "en"
	}
}
