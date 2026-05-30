package scraper

import (
	"GameLibrary/internal/config"
	"GameLibrary/internal/game"
	"GameLibrary/internal/logger"
)

type Result struct {
	Title             string            `json:"title"`
	TitleNative       string            `json:"titleNative"`
	Description       string            `json:"description"`
	Developer         string            `json:"developer"`
	Publisher         string            `json:"publisher"`
	ReleaseDate       string            `json:"releaseDate"`
	Tags              []string          `json:"tags"`
	CoverURL          string            `json:"coverUrl"`
	CoverLandscapeURL string            `json:"-"`
	Links             map[string]string `json:"links"`
}

type Source interface {
	Key() string
	Search(gameDir string, appID string) (*Result, error)
}

type Pipeline struct {
	config *config.Config
	source map[string]Source
}

func NewPipeline(cfg *config.Config) *Pipeline {
	return &Pipeline{
		config: cfg,
		source: make(map[string]Source),
	}
}

func (p *Pipeline) Register(src Source) {
	p.source[src.Key()] = src
}

func (p *Pipeline) Scrape(gameDir string, gameInfo *game.GameInfo) (*Result, string, error) {
	logger.ScrapeStarted(gameInfo.ID, gameInfo.Title)

	for _, srcCfg := range p.config.Sources {
		if !srcCfg.Enabled {
			logger.ScrapeSourceSkipped(gameInfo.ID, srcCfg.Key, "disabled")
			continue
		}
		scraper, ok := p.source[srcCfg.Key]
		if !ok {
			logger.ScrapeSourceSkipped(gameInfo.ID, srcCfg.Key, "not registered")
			continue
		}

		logger.ScrapeSourceAttempt(gameInfo.ID, srcCfg.Key, gameInfo.Title)

		result, err := scraper.Search(gameDir, gameInfo.PlatformID)
		if err != nil {
			logger.ScrapeSourceFailed(gameInfo.ID, gameInfo.Title, srcCfg.Key, err)
			continue
		}
		if result == nil {
			logger.ScrapeSourceEmpty(gameInfo.ID, gameInfo.Title, srcCfg.Key)
			continue
		}

		logger.ScrapeSuccess(gameInfo.ID, gameInfo.Title, srcCfg.Key, result.Title, gameInfo.PlatformID)

		return result, srcCfg.Key, nil
	}

	logger.ScrapeAllSourcesFailed(gameInfo.ID, gameInfo.Title)

	return nil, "", nil
}

func ApplyResult(info *game.GameInfo, result *Result, sourceKey string) {
	info.Title = result.Title
	info.TitleNative = result.TitleNative
	info.Platform = sourceKey
	if result.Links != nil {
		if id, ok := result.Links["platformId"]; ok {
			info.PlatformID = id
		}
	}
	info.Metadata = &game.Metadata{
		CoverURL:       "cover",
		CoverLandscape: "cover_landscape",
		ReleaseDate:    result.ReleaseDate,
		Developer:      result.Developer,
		Publisher:      result.Publisher,
		Tags:           result.Tags,
		Description:    result.Description,
		Links:          result.Links,
	}
}
