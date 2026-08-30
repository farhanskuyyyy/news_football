package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/farhanarfianto/apigo-docker/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// perPage is the number of items per page requested from Sportmonks.
const perPage = 50

// fixtureDetailBackfillLimit caps how many fixtures per sync run are enriched
// per-fixture for lineup details (bounds quota usage).
const fixtureDetailBackfillLimit = 80

// playerStatsLimit caps how many players per sync run get their season
// statistics fetched (one request each).
const playerStatsLimit = 150

// dbBatchSize is the number of records to upsert per DB transaction.
const dbBatchSize = 200

// Dynamic TTL Constants for Table Synchronization (saving Sportmonks rate limits & quota)
const (
	TTLFixtures    = 1 * time.Hour
	TTLStandings   = 1 * time.Hour
	TTLTopscorers  = 1 * time.Hour
	TTLTransfers   = 12 * time.Hour
	TTLSquads      = 24 * time.Hour
	TTLPlayers     = 24 * time.Hour
	TTLPlayerStats = 12 * time.Hour
	TTLRounds      = 24 * time.Hour
	TTLStages      = 24 * time.Hour
	TTLTeams       = 7 * 24 * time.Hour
	TTLSeasons     = 7 * 24 * time.Hour
	TTLVenues      = 30 * 24 * time.Hour
	TTLCoaches     = 30 * 24 * time.Hour
	TTLReferees    = 30 * 24 * time.Hour
	TTLRivals      = 30 * 24 * time.Hour
	TTLStates      = 30 * 24 * time.Hour
	TTLLeagues     = 30 * 24 * time.Hour
)

type FootballResponseEnvelope[T any] struct {
	Data       []T             `json:"data"`
	Pagination *PaginationInfo `json:"pagination"`
}

type FootballScrapeResult struct {
	ActiveLeaguesCount int `json:"active_leagues_count"`
	Leagues            int `json:"leagues,omitempty"`
	Seasons            int `json:"seasons"`
	Stages             int `json:"stages"`
	Rounds             int `json:"rounds"`
	Teams              int `json:"teams"`
	Squads             int `json:"squads"`
	Players            int `json:"players"`
	Fixtures           int `json:"fixtures"`
	LineupDetails      int `json:"lineup_details"`
	PlayerStatistics   int `json:"player_statistics"`
	Venues             int `json:"venues"`
	Coaches            int `json:"coaches"`
	Referees           int `json:"referees"`
	Standings          int `json:"standings"`
	Topscorers         int `json:"topscorers"`
	Transfers          int `json:"transfers"`
	Rivals             int `json:"rivals"`
	States             int `json:"states"`
}

type FootballScraper struct {
	DB     *gorm.DB
	Client *SportmonksClient
}

func NewFootballScraper(db *gorm.DB, client *SportmonksClient) *FootballScraper {
	return &FootballScraper{
		DB:     db,
		Client: client,
	}
}

// ShouldSync checks if a table needs to be synchronized based on dynamic TTL.
// Returns false (skips) if last synced within TTL and status was success.
func (s *FootballScraper) ShouldSync(tableName string, defaultTTL time.Duration, force ...bool) bool {
	if len(force) > 0 && force[0] {
		return true
	}

	var syncRecord models.SyncTable
	if err := s.DB.First(&syncRecord, "table_name = ?", tableName).Error; err != nil {
		return true // Never synced before
	}

	ttl := defaultTTL
	if syncRecord.IntervalSeconds > 0 {
		ttl = time.Duration(syncRecord.IntervalSeconds) * time.Second
	}

	if time.Since(syncRecord.LatestSyncedAt) < ttl && syncRecord.Status == "success" {
		log.Printf("[SyncTracker] SKIPPING '%s': last synced %v ago (TTL: %v)",
			tableName, time.Since(syncRecord.LatestSyncedAt).Round(time.Second), ttl)
		return false
	}
	return true
}

// MarkSynced records the sync execution in the sync_tables table.
func (s *FootballScraper) MarkSynced(tableName string, recordCount int, defaultTTL time.Duration, syncErr error) {
	status := "success"
	errMsg := ""
	if syncErr != nil {
		status = "failed"
		errMsg = syncErr.Error()
	}

	syncRecord := models.SyncTable{
		TableName:       tableName,
		LatestSyncedAt:  time.Now(),
		IntervalSeconds: int(defaultTTL.Seconds()),
		RecordsSynced:   recordCount,
		Status:          status,
		ErrorMessage:    errMsg,
		UpdatedAt:       time.Now(),
	}

	_ = s.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "table_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"latest_synced_at", "interval_seconds", "records_synced", "status", "error_message", "updated_at"}),
	}).Create(&syncRecord)
}

// GetCurrentSeasonIDs returns active/current season IDs for active leagues.
// If specificSeasonID is provided, it targets only that season.
func (s *FootballScraper) GetCurrentSeasonIDs(activeLeagueIDs map[uint]bool, specificSeasonID ...uint) map[uint]bool {
	seasonIDMap := make(map[uint]bool)
	if len(specificSeasonID) > 0 && specificSeasonID[0] > 0 {
		seasonIDMap[specificSeasonID[0]] = true
		return seasonIDMap
	}

	var seasons []models.Season
	query := s.DB.Where("is_current = ?", true)
	if len(activeLeagueIDs) > 0 {
		var ids []uint
		for id := range activeLeagueIDs {
			ids = append(ids, id)
		}
		query = query.Where("league_id IN ?", ids)
	}
	query.Find(&seasons)

	for _, sn := range seasons {
		seasonIDMap[sn.ID] = true
	}

	// Fallback if is_current is not yet set in DB: pick latest season per active league
	if len(seasonIDMap) == 0 {
		var allSeasons []models.Season
		q := s.DB
		if len(activeLeagueIDs) > 0 {
			var ids []uint
			for id := range activeLeagueIDs {
				ids = append(ids, id)
			}
			q = q.Where("league_id IN ?", ids)
		}
		q.Order("ending_at DESC, id DESC").Find(&allSeasons)
		seenLeague := make(map[uint]bool)
		for _, sn := range allSeasons {
			if !seenLeague[sn.LeagueID] {
				seenLeague[sn.LeagueID] = true
				seasonIDMap[sn.ID] = true
			}
		}
	}

	return seasonIDMap
}

// extractCursor parses a cursor token if raw is a full URL.
func extractCursor(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "cursor=") {
		if u, err := url.Parse(raw); err == nil {
			if c := u.Query().Get("cursor"); c != "" {
				return c
			}
		}
	}
	return raw
}

// ScrapeLeagues fetches all leagues from Sportmonks and populates the DB setting Status = true by default.
func (s *FootballScraper) ScrapeLeagues() (int, error) {
	count, err := scrapePaginated(s.Client, s.DB, "football/leagues", nil, func(data []models.League) (int, error) {
		for i := range data {
			data[i].Status = true
		}
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
		return len(data), err
	})
	s.MarkSynced("leagues", count, TTLLeagues, err)
	return count, err
}

// ScrapeAllFootball orchestrates scraping across all football sub-entities and pivot tables.
// By default it only syncs tables whose TTL has expired, and strictly targets the CURRENT season.
func (s *FootballScraper) ScrapeAllFootball(force ...bool) (*FootballScrapeResult, error) {
	isForce := len(force) > 0 && force[0]
	result := &FootballScrapeResult{}

	// Query active leagues from DB (status = true AND active = true)
	var activeLeagues []models.League
	if err := s.DB.Where("status = ? AND active = ?", true, true).Find(&activeLeagues).Error; err != nil {
		return nil, fmt.Errorf("failed to query active leagues from database: %w", err)
	}

	result.ActiveLeaguesCount = len(activeLeagues)
	if len(activeLeagues) == 0 {
		log.Println("[FootballScraper] No active leagues found (status = true AND active = true). Run /sportmonks/scrape/leagues first or set status = true.")
		return result, nil
	}

	activeLeagueIDs := make(map[uint]bool)
	for _, l := range activeLeagues {
		activeLeagueIDs[l.ID] = true
	}
	log.Printf("[FootballScraper] Scraping data for %d active leagues (Force: %v)", len(activeLeagues), isForce)

	var err error

	// 1. Seasons (TTL: 7 days)
	if s.ShouldSync("seasons", TTLSeasons, isForce) {
		result.Seasons, err = s.ScrapeSeasons(activeLeagueIDs)
		s.MarkSynced("seasons", result.Seasons, TTLSeasons, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping seasons: %v", err)
		}
	}

	// Resolve Current Season IDs for downstream filtering
	currentSeasonIDs := s.GetCurrentSeasonIDs(activeLeagueIDs)
	log.Printf("[FootballScraper] Target current seasons: %d active seasons", len(currentSeasonIDs))

	// 2. Stages (TTL: 24h, filtered by current seasons)
	if s.ShouldSync("stages", TTLStages, isForce) {
		result.Stages, err = s.ScrapeStages(activeLeagueIDs, currentSeasonIDs)
		s.MarkSynced("stages", result.Stages, TTLStages, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping stages: %v", err)
		}
	}

	// 3. Rounds (TTL: 24h, filtered by current seasons)
	if s.ShouldSync("rounds", TTLRounds, isForce) {
		result.Rounds, err = s.ScrapeRounds(activeLeagueIDs, currentSeasonIDs)
		s.MarkSynced("rounds", result.Rounds, TTLRounds, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping rounds: %v", err)
		}
	}

	// 4. Teams (TTL: 7 days, teams for current seasons)
	if s.ShouldSync("teams", TTLTeams, isForce) {
		result.Teams, err = s.ScrapeTeams(activeLeagueIDs, currentSeasonIDs)
		s.MarkSynced("teams", result.Teams, TTLTeams, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping teams: %v", err)
		}
	}

	// 5. Standings (TTL: 1h, filtered by current seasons)
	if s.ShouldSync("standings", TTLStandings, isForce) {
		result.Standings, err = s.ScrapeStandings(activeLeagueIDs, currentSeasonIDs)
		s.MarkSynced("standings", result.Standings, TTLStandings, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping standings: %v", err)
		}
	}

	// 6. Squads & Players (TTL: 24h, filtered by current seasons)
	if s.ShouldSync("squads", TTLSquads, isForce) {
		result.Squads, err = s.ScrapeSquads(activeLeagueIDs, currentSeasonIDs)
		s.MarkSynced("squads", result.Squads, TTLSquads, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping squads: %v", err)
		}
	}

	// 7. Players (Extended profile per klub aktif, TTL: 24h)
	if s.ShouldSync("players", TTLPlayers, isForce) {
		result.Players, err = s.ScrapePlayers(activeLeagueIDs, currentSeasonIDs)
		s.MarkSynced("players", result.Players, TTLPlayers, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping players: %v", err)
		}
	}

	// 7b. Player season statistics (TTL: 12h) — one request per player, bounded.
	if s.ShouldSync("player_statistics", TTLPlayerStats, isForce) {
		result.PlayerStatistics, err = s.ScrapePlayerStatistics(activeLeagueIDs, currentSeasonIDs)
		s.MarkSynced("player_statistics", result.PlayerStatistics, TTLPlayerStats, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping player statistics: %v", err)
		}
	}

	// 8. Fixtures (TTL: 1h, filtered by current seasons)
	if s.ShouldSync("fixtures", TTLFixtures, isForce) {
		result.Fixtures, err = s.ScrapeFixtures(activeLeagueIDs, currentSeasonIDs)
		s.MarkSynced("fixtures", result.Fixtures, TTLFixtures, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping fixtures: %v", err)
		}
	}

	// 8b. Lineup-detail backfill (TTL: 1h) — per-fixture, only for fixtures whose
	// details the list endpoint dropped.
	if s.ShouldSync("fixture_lineup_details", TTLFixtures, isForce) {
		result.LineupDetails, err = s.ScrapeFixtureLineupDetails(currentSeasonIDs)
		s.MarkSynced("fixture_lineup_details", result.LineupDetails, TTLFixtures, err)
		if err != nil {
			log.Printf("[FootballScraper] Error backfilling lineup details: %v", err)
		}
	}

	// 9. Venues (TTL: 30 days, scoped to current seasons)
	if s.ShouldSync("venues", TTLVenues, isForce) {
		result.Venues, err = s.ScrapeVenues(currentSeasonIDs)
		s.MarkSynced("venues", result.Venues, TTLVenues, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping venues: %v", err)
		}
	}

	// 10. Coaches (TTL: 30 days)
	if s.ShouldSync("coaches", TTLCoaches, isForce) {
		result.Coaches, err = s.ScrapeCoaches()
		s.MarkSynced("coaches", result.Coaches, TTLCoaches, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping coaches: %v", err)
		}
	}

	// 11. Referees (TTL: 30 days, scoped to current seasons)
	if s.ShouldSync("referees", TTLReferees, isForce) {
		result.Referees, err = s.ScrapeReferees(currentSeasonIDs)
		s.MarkSynced("referees", result.Referees, TTLReferees, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping referees: %v", err)
		}
	}

	// 12. Topscorers (TTL: 1h, filtered by current seasons)
	if s.ShouldSync("topscorers", TTLTopscorers, isForce) {
		result.Topscorers, err = s.ScrapeTopscorers(activeLeagueIDs, currentSeasonIDs)
		s.MarkSynced("topscorers", result.Topscorers, TTLTopscorers, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping topscorers: %v", err)
		}
	}

	// 13. Rivals (TTL: 30 days)
	if s.ShouldSync("rivals", TTLRivals, isForce) {
		result.Rivals, err = s.ScrapeRivals()
		s.MarkSynced("rivals", result.Rivals, TTLRivals, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping rivals: %v", err)
		}
	}

	// 14. Transfers (TTL: 12h, scoped to active teams)
	if s.ShouldSync("transfers", TTLTransfers, isForce) {
		result.Transfers, err = s.ScrapeTransfers(activeLeagueIDs, currentSeasonIDs)
		s.MarkSynced("transfers", result.Transfers, TTLTransfers, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping transfers: %v", err)
		}
	}

	// 15. States (TTL: 30 days)
	if s.ShouldSync("states", TTLStates, isForce) {
		result.States, err = s.ScrapeStates()
		s.MarkSynced("states", result.States, TTLStates, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping states: %v", err)
		}
	}

	return result, nil
}

// -----------------------------------------------------------------------------
// Individual Scraping Methods (Supporting Current Season Filtering)
// -----------------------------------------------------------------------------

// ScrapeSeasons fetches seasons and filters them by active leagues.
func (s *FootballScraper) ScrapeSeasons(activeLeagueIDs map[uint]bool) (int, error) {
	return scrapePaginated(s.Client, s.DB, "football/seasons", nil, func(data []models.Season) (int, error) {
		var filtered []models.Season
		for _, season := range data {
			if activeLeagueIDs == nil || activeLeagueIDs[season.LeagueID] {
				filtered = append(filtered, season)
			}
		}
		if len(filtered) == 0 {
			return 0, nil
		}
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&filtered, dbBatchSize).Error
		return len(filtered), err
	})
}

// ScrapeStages fetches stages and filters them by active leagues & current seasons.
func (s *FootballScraper) ScrapeStages(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) (int, error) {
	var targetSeasons map[uint]bool
	if len(currentSeasonIDs) > 0 {
		targetSeasons = currentSeasonIDs[0]
	}

	return scrapePaginated(s.Client, s.DB, "football/stages", nil, func(data []models.Stage) (int, error) {
		var filtered []models.Stage
		for _, stage := range data {
			if activeLeagueIDs != nil && !activeLeagueIDs[stage.LeagueID] {
				continue
			}
			if targetSeasons != nil && len(targetSeasons) > 0 && !targetSeasons[stage.SeasonID] {
				continue
			}
			filtered = append(filtered, stage)
		}
		if len(filtered) == 0 {
			return 0, nil
		}
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&filtered, dbBatchSize).Error
		return len(filtered), err
	})
}

// ScrapeRounds fetches rounds and filters them by active leagues & current seasons.
func (s *FootballScraper) ScrapeRounds(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) (int, error) {
	var targetSeasons map[uint]bool
	if len(currentSeasonIDs) > 0 {
		targetSeasons = currentSeasonIDs[0]
	}

	return scrapePaginated(s.Client, s.DB, "football/rounds", nil, func(data []models.Round) (int, error) {
		var filtered []models.Round
		for _, round := range data {
			if activeLeagueIDs != nil && !activeLeagueIDs[round.LeagueID] {
				continue
			}
			if targetSeasons != nil && len(targetSeasons) > 0 && !targetSeasons[round.SeasonID] {
				continue
			}
			filtered = append(filtered, round)
		}
		if len(filtered) == 0 {
			return 0, nil
		}
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&filtered, dbBatchSize).Error
		return len(filtered), err
	})
}

// ScrapeTeams fetches teams only for current seasons belonging to active leagues (via football/teams/seasons/:seasonId).
// TeamCoachInclude decodes one item of the team `coaches` include (the pivot
// plus the nested coach entity).
type TeamCoachInclude struct {
	TeamID  uint          `json:"team_id"`
	CoachID uint          `json:"coach_id"`
	Active  bool          `json:"active"`
	Start   *string       `json:"start"`
	End     *string       `json:"end"`
	Coach   *models.Coach `json:"coach"`
}

// TeamWithCoachesPayload decodes a team together with its coaches include.
type TeamWithCoachesPayload struct {
	models.Team
	Coaches []TeamCoachInclude `json:"coaches"`
}

func (s *FootballScraper) ScrapeTeams(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) (int, error) {
	var targetSeasons map[uint]bool
	if len(currentSeasonIDs) > 0 {
		targetSeasons = currentSeasonIDs[0]
	} else {
		targetSeasons = s.GetCurrentSeasonIDs(activeLeagueIDs)
	}

	var seasonIDs []uint
	for id := range targetSeasons {
		seasonIDs = append(seasonIDs, id)
	}

	var seasons []models.Season
	query := s.DB
	if len(seasonIDs) > 0 {
		query = query.Where("id IN ?", seasonIDs)
	} else if len(activeLeagueIDs) > 0 {
		var ids []uint
		for id := range activeLeagueIDs {
			ids = append(ids, id)
		}
		query = query.Where("league_id IN ?", ids)
	}
	if err := query.Find(&seasons).Error; err != nil {
		return 0, fmt.Errorf("failed to query seasons for teams: %w", err)
	}

	if len(seasons) == 0 {
		log.Println("[FootballScraper] No active seasons found in DB to scrape teams for.")
		return 0, nil
	}

	log.Printf("[FootballScraper] Scraping teams for %d current/active seasons...", len(seasons))

	extraParams := map[string]string{
		"include": "coaches.coach",
	}
	var total int
	for _, season := range seasons {
		currentSeasonID := season.ID
		endpoint := fmt.Sprintf("football/teams/seasons/%d", season.ID)
		count, err := scrapePaginated(s.Client, s.DB, endpoint, extraParams, func(data []TeamWithCoachesPayload) (int, error) {
			// Upsert the plain team rows
			teams := make([]models.Team, 0, len(data))
			for _, item := range data {
				teams = append(teams, item.Team)
			}
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&teams, dbBatchSize).Error; err != nil {
				return 0, err
			}

			// Populate SeasonTeam pivot + TeamCoach link + Coach entities
			var seasonTeams []models.SeasonTeam
			var teamCoaches []models.TeamCoach
			var coaches []models.Coach
			for _, item := range data {
				seasonTeams = append(seasonTeams, models.SeasonTeam{
					SeasonID: currentSeasonID,
					TeamID:   item.ID,
				})
				for _, ch := range item.Coaches {
					if ch.CoachID == 0 {
						continue
					}
					teamCoaches = append(teamCoaches, models.TeamCoach{
						TeamID:  item.ID,
						CoachID: ch.CoachID,
						Active:  ch.Active,
						Start:   ch.Start,
						End:     ch.End,
					})
					if ch.Coach != nil && ch.Coach.ID > 0 {
						coaches = append(coaches, *ch.Coach)
					}
				}
			}
			if len(seasonTeams) > 0 {
				_ = s.DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&seasonTeams, dbBatchSize)
			}
			if len(coaches) > 0 {
				_ = s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&coaches, dbBatchSize)
			}
			if len(teamCoaches) > 0 {
				_ = s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&teamCoaches, dbBatchSize)
			}

			return len(data), nil
		})
		if err != nil {
			log.Printf("[FootballScraper] Notice: teams for season %d: %v", season.ID, err)
		} else {
			total += count
		}
	}
	return total, nil
}

// getActiveSeasonTeams retrieves distinct (season_id, team_id) pairs for active leagues & current seasons.
func (s *FootballScraper) getActiveSeasonTeams(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) []models.SeasonTeam {
	var targetSeasonIDs []uint
	if len(currentSeasonIDs) > 0 && len(currentSeasonIDs[0]) > 0 {
		for id := range currentSeasonIDs[0] {
			targetSeasonIDs = append(targetSeasonIDs, id)
		}
	} else if len(activeLeagueIDs) > 0 {
		var leagueIDs []uint
		for id := range activeLeagueIDs {
			leagueIDs = append(leagueIDs, id)
		}
		s.DB.Model(&models.Season{}).Where("league_id IN ? AND is_current = ?", leagueIDs, true).Pluck("id", &targetSeasonIDs)
		if len(targetSeasonIDs) == 0 {
			s.DB.Model(&models.Season{}).Where("league_id IN ?", leagueIDs).Pluck("id", &targetSeasonIDs)
		}
	}

	var rawPairs []models.SeasonTeam

	// 1. Check SeasonTeam table
	query := s.DB.Model(&models.SeasonTeam{})
	if len(targetSeasonIDs) > 0 {
		query = query.Where("season_id IN ?", targetSeasonIDs)
	}
	query.Select("DISTINCT season_id, team_id").Find(&rawPairs)

	// 2. Check Standing table for distinct (season_id, participant_id)
	if len(rawPairs) == 0 {
		var standings []models.Standing
		stQuery := s.DB.Model(&models.Standing{})
		if len(targetSeasonIDs) > 0 {
			stQuery = stQuery.Where("season_id IN ?", targetSeasonIDs)
		}
		stQuery.Select("DISTINCT season_id, participant_id").Find(&standings)
		for _, st := range standings {
			if st.SeasonID > 0 && st.ParticipantID > 0 {
				rawPairs = append(rawPairs, models.SeasonTeam{
					SeasonID: st.SeasonID,
					TeamID:   st.ParticipantID,
				})
			}
		}
	}

	// 3. Fallback: pair active seasons with all existing teams
	if len(rawPairs) == 0 && len(targetSeasonIDs) > 0 {
		var teamIDs []uint
		s.DB.Model(&models.Team{}).Pluck("id", &teamIDs)
		for _, snID := range targetSeasonIDs {
			for _, tmID := range teamIDs {
				rawPairs = append(rawPairs, models.SeasonTeam{
					SeasonID: snID,
					TeamID:   tmID,
				})
			}
		}
	}

	// Deduplicate in memory
	seen := make(map[string]bool)
	var distinct []models.SeasonTeam
	for _, p := range rawPairs {
		key := fmt.Sprintf("%d-%d", p.SeasonID, p.TeamID)
		if !seen[key] && p.SeasonID > 0 && p.TeamID > 0 {
			seen[key] = true
			distinct = append(distinct, p)
		}
	}

	return distinct
}

// SquadWithPlayerPayload decodes squad records with nested player details.
type SquadWithPlayerPayload struct {
	models.Squad
	Player *models.Player `json:"player"`
}

// ScrapeSquads fetches squads per distinct (season_id, team_id) and populates squads, players, and player_seasons.
func (s *FootballScraper) ScrapeSquads(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) (int, error) {
	seasonTeams := s.getActiveSeasonTeams(activeLeagueIDs, currentSeasonIDs...)
	if len(seasonTeams) == 0 {
		log.Println("[FootballScraper] No active (season, team) pairs found to scrape squads for.")
		return 0, nil
	}

	log.Printf("[FootballScraper] Scraping squads for %d distinct (season, team) pairs...", len(seasonTeams))

	extraParams := map[string]string{"include": "player"}
	var total int

	for _, st := range seasonTeams {
		currentSeasonID := st.SeasonID
		currentTeamID := st.TeamID
		endpoint := fmt.Sprintf("football/squads/seasons/%d/teams/%d", st.SeasonID, st.TeamID)

		count, err := scrapePaginated(s.Client, s.DB, endpoint, extraParams, func(data []SquadWithPlayerPayload) (int, error) {
			var squads []models.Squad
			var players []models.Player
			var playerSeasons []models.PlayerSeason

			for _, item := range data {
				sq := item.Squad
				sq.SeasonID = &currentSeasonID
				sq.TeamID = currentTeamID
				squads = append(squads, sq)

				if item.Player != nil && item.Player.ID > 0 {
					players = append(players, *item.Player)
				}

				if sq.PlayerID > 0 {
					playerSeasons = append(playerSeasons, models.PlayerSeason{
						PlayerID: sq.PlayerID,
						SeasonID: currentSeasonID,
						TeamID:   currentTeamID,
					})
				}
			}

			if len(squads) > 0 {
				_ = s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&squads, dbBatchSize)
			}
			if len(players) > 0 {
				_ = s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&players, dbBatchSize)
			}
			if len(playerSeasons) > 0 {
				_ = s.DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&playerSeasons, dbBatchSize)
			}

			return len(data), nil
		})

		if err != nil {
			log.Printf("[FootballScraper] Notice: squad for season %d, team %d: %v", st.SeasonID, st.TeamID, err)
		} else {
			total += count
		}
	}
	return total, nil
}

// ScrapePlayers fetches extended player profiles for distinct teams in active leagues.
func (s *FootballScraper) ScrapePlayers(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) (int, error) {
	seasonTeams := s.getActiveSeasonTeams(activeLeagueIDs, currentSeasonIDs...)
	teamIDMap := make(map[uint]bool)
	for _, st := range seasonTeams {
		teamIDMap[st.TeamID] = true
	}

	if len(teamIDMap) == 0 {
		var allTeams []models.Team
		s.DB.Model(&models.Team{}).Find(&allTeams)
		for _, tm := range allTeams {
			teamIDMap[tm.ID] = true
		}
	}

	if len(teamIDMap) == 0 {
		log.Println("[FootballScraper] No teams found in DB to scrape players for.")
		return 0, nil
	}

	log.Printf("[FootballScraper] Scraping extended players for %d distinct active teams...", len(teamIDMap))

	var total int
	for teamID := range teamIDMap {
		endpoint := fmt.Sprintf("football/squads/teams/%d/extended", teamID)
		count, err := scrapePaginated(s.Client, s.DB, endpoint, nil, func(data []models.Player) (int, error) {
			err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
			return len(data), err
		})
		if err != nil {
			log.Printf("[FootballScraper] Notice: extended squad for team %d: %v", teamID, err)
		} else {
			total += count
		}
	}
	return total, nil
}

// LineupWithDetailsPayload decodes a lineup with its nested player details/statistics.
type LineupWithDetailsPayload struct {
	models.FixtureLineup
	Details       []models.FixtureLineupDetail `json:"details"`
	LineupDetails []models.FixtureLineupDetail `json:"lineup_details"`
	LineupDetail  []models.FixtureLineupDetail `json:"lineupDetail"`
}

// FixturePayload decodes a fixture along with its nested includes.
type FixturePayload struct {
	models.Fixture
	Events     []models.FixtureEvent      `json:"events"`
	Lineups    []LineupWithDetailsPayload `json:"lineups"`
	Scores     []models.FixtureScore      `json:"scores"`
	Statistics []models.FixtureStatistic  `json:"statistics"`
	Referees   []struct {
		ID        uint  `json:"id"`
		RefereeID uint  `json:"referee_id"`
		TypeID    *uint `json:"type_id"`
	} `json:"referees"`
	Participants []struct {
		ID uint `json:"id"`
	} `json:"participants"`
}

// ScrapeFixtures fetches fixtures and nested sub-entities (events, lineups with details, statistics, scores, referees).
func (s *FootballScraper) ScrapeFixtures(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) (int, error) {
	var targetSeasons map[uint]bool
	if len(currentSeasonIDs) > 0 {
		targetSeasons = currentSeasonIDs[0]
	}

	extraParams := map[string]string{
		"include": "events;lineups.details;scores;statistics;referees;participants",
	}

	return scrapePaginated(s.Client, s.DB, "football/fixtures", extraParams, func(data []FixturePayload) (int, error) {
		var fixtures []models.Fixture
		var events []models.FixtureEvent
		var lineups []models.FixtureLineup
		var lineupDetails []models.FixtureLineupDetail
		var scores []models.FixtureScore
		var stats []models.FixtureStatistic
		var referees []models.FixtureReferee
		var seasonTeams []models.SeasonTeam

		for _, item := range data {
			if activeLeagueIDs != nil && !activeLeagueIDs[item.LeagueID] {
				continue
			}
			if targetSeasons != nil && len(targetSeasons) > 0 && !targetSeasons[item.SeasonID] {
				continue
			}

			fixtures = append(fixtures, item.Fixture)

			for _, ev := range item.Events {
				ev.FixtureID = item.ID
				events = append(events, ev)
			}
			for _, lu := range item.Lineups {
				lModel := lu.FixtureLineup
				lModel.FixtureID = item.ID
				lineups = append(lineups, lModel)

				allDetails := append(lu.Details, lu.LineupDetails...)
				allDetails = append(allDetails, lu.LineupDetail...)
				for _, dt := range allDetails {
					dt.FixtureID = item.ID
					dt.LineupID = lModel.ID
					if dt.PlayerID == 0 {
						dt.PlayerID = lModel.PlayerID
					}
					lineupDetails = append(lineupDetails, dt)
				}
			}
			for _, sc := range item.Scores {
				sc.FixtureID = item.ID
				scores = append(scores, sc)
			}
			for _, st := range item.Statistics {
				st.FixtureID = item.ID
				stats = append(stats, st)
			}
			for _, rf := range item.Referees {
				refID := rf.RefereeID
				if refID == 0 {
					refID = rf.ID
				}
				referees = append(referees, models.FixtureReferee{
					FixtureID: item.ID,
					RefereeID: refID,
					TypeID:    rf.TypeID,
				})
			}
			for _, pt := range item.Participants {
				if item.SeasonID > 0 && pt.ID > 0 {
					seasonTeams = append(seasonTeams, models.SeasonTeam{
						SeasonID: item.SeasonID,
						TeamID:   pt.ID,
					})
				}
			}
		}

		if len(fixtures) == 0 {
			return 0, nil
		}

		if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&fixtures, dbBatchSize).Error; err != nil {
			return 0, err
		}

		// Save sub-entities & pivot tables
		if len(events) > 0 {
			_ = s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&events, dbBatchSize)
		}
		if len(lineups) > 0 {
			_ = s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&lineups, dbBatchSize)
		}
		if len(lineupDetails) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&lineupDetails, dbBatchSize).Error; err != nil {
				log.Printf("[FootballScraper] Error saving %d lineup_details: %v", len(lineupDetails), err)
			}
		} else if len(lineups) > 0 {
			// Lineups present but the list endpoint returned no nested details —
			// they will be backfilled per-fixture by ScrapeFixtureLineupDetails.
			log.Printf("[FootballScraper] %d lineups parsed but 0 lineup_details from list endpoint (will backfill per-fixture)", len(lineups))
		}
		if len(scores) > 0 {
			_ = s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&scores, dbBatchSize)
		}
		if len(stats) > 0 {
			_ = s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&stats, dbBatchSize)
		}
		if len(referees) > 0 {
			_ = s.DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&referees, dbBatchSize)
		}
		if len(seasonTeams) > 0 {
			_ = s.DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&seasonTeams, dbBatchSize)
		}

		return len(fixtures), nil
	})
}

// ScrapeFixtureLineupDetails backfills per-player lineup details (ratings,
// minutes, shots, passes, etc.) for fixtures that already have lineups but no
// details. Sportmonks' GET-ALL /fixtures endpoint frequently omits the 2nd-level
// `lineups.details` nesting, so we fetch those fixtures individually. Bounded by
// fixtureDetailBackfillLimit per run to protect rate limits.
func (s *FootballScraper) ScrapeFixtureLineupDetails(currentSeasonIDs ...map[uint]bool) (int, error) {
	seasonIDs := seasonIDsFromMap(currentSeasonIDs...)

	q := s.DB.Model(&models.Fixture{}).
		Where("EXISTS (SELECT 1 FROM fixture_lineups l WHERE l.fixture_id = fixtures.id)").
		Where("NOT EXISTS (SELECT 1 FROM fixture_lineup_details d WHERE d.fixture_id = fixtures.id)")
	if len(seasonIDs) > 0 {
		q = q.Where("season_id IN ?", seasonIDs)
	}
	var fixtureIDs []uint
	q.Order("starting_at DESC").Limit(fixtureDetailBackfillLimit).Pluck("id", &fixtureIDs)

	if len(fixtureIDs) == 0 {
		return 0, nil
	}
	log.Printf("[FootballScraper] Backfilling lineup details for %d fixtures (per-fixture)...", len(fixtureIDs))

	total := 0
	for _, fid := range fixtureIDs {
		raw, err := s.Client.Get(fmt.Sprintf("football/fixtures/%d", fid), map[string]string{
			"include": "lineups.details.type",
		})
		if err != nil {
			log.Printf("[FootballScraper] Notice: lineup-detail backfill fixture %d: %v", fid, err)
			continue
		}

		var env struct {
			Data FixturePayload `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			log.Printf("[FootballScraper] Notice: parse fixture %d: %v", fid, err)
			continue
		}

		var details []models.FixtureLineupDetail
		for _, lu := range env.Data.Lineups {
			allDetails := append(lu.Details, lu.LineupDetails...)
			allDetails = append(allDetails, lu.LineupDetail...)
			for _, dt := range allDetails {
				dt.FixtureID = fid
				dt.LineupID = lu.FixtureLineup.ID
				if dt.PlayerID == 0 {
					dt.PlayerID = lu.FixtureLineup.PlayerID
				}
				details = append(details, dt)
			}
		}
		if len(details) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&details, dbBatchSize).Error; err != nil {
				log.Printf("[FootballScraper] Error saving lineup details for fixture %d: %v", fid, err)
			} else {
				total += len(details)
			}
		}

		time.Sleep(350 * time.Millisecond)
	}

	log.Printf("[FootballScraper] Lineup-detail backfill: saved %d detail rows", total)
	return total, nil
}

// FixtureDetailResult summarises a full-detail fixture init run.
type FixtureDetailResult struct {
	FixturesProcessed int `json:"fixtures_processed"`
	Events            int `json:"events"`
	Lineups           int `json:"lineups"`
	LineupDetails     int `json:"lineup_details"`
	Statistics        int `json:"statistics"`
	Scores            int `json:"scores"`
}

// ScrapeFixtureDetails fetches FULL per-fixture detail (events, lineups + their
// per-player details, statistics, scores) for fixtures belonging to active
// leagues & current seasons. Meant for INITIAL seeding: the GET-ALL /fixtures
// list endpoint drops deep nested includes, so full detail must be fetched one
// fixture at a time.
//
// specificSeasonID > 0 targets a single season. limit <= 0 means no cap. When
// force is false, fixtures that already have events are skipped (resumable).
func (s *FootballScraper) ScrapeFixtureDetails(limit int, force bool, specificSeasonID uint) (*FixtureDetailResult, error) {
	result := &FixtureDetailResult{}

	var activeLeagues []models.League
	if err := s.DB.Where("status = ? AND active = ?", true, true).Find(&activeLeagues).Error; err != nil {
		return nil, fmt.Errorf("failed to query active leagues: %w", err)
	}
	if len(activeLeagues) == 0 {
		log.Println("[FootballScraper] ScrapeFixtureDetails: no active leagues.")
		return result, nil
	}
	activeLeagueIDs := make(map[uint]bool)
	for _, l := range activeLeagues {
		activeLeagueIDs[l.ID] = true
	}

	var currentSeasons map[uint]bool
	if specificSeasonID > 0 {
		currentSeasons = map[uint]bool{specificSeasonID: true}
	} else {
		currentSeasons = s.GetCurrentSeasonIDs(activeLeagueIDs)
	}
	seasonIDs := seasonIDsFromMap(currentSeasons)
	if len(seasonIDs) == 0 {
		log.Println("[FootballScraper] ScrapeFixtureDetails: no current seasons resolved.")
		return result, nil
	}

	// Candidate fixtures
	q := s.DB.Model(&models.Fixture{}).Where("season_id IN ?", seasonIDs)
	if !force {
		q = q.Where("NOT EXISTS (SELECT 1 FROM fixture_events e WHERE e.fixture_id = fixtures.id)")
	}
	q = q.Order("starting_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var fixtureIDs []uint
	q.Pluck("id", &fixtureIDs)

	if len(fixtureIDs) == 0 {
		log.Println("[FootballScraper] ScrapeFixtureDetails: no fixtures to process.")
		return result, nil
	}
	log.Printf("[FootballScraper] ScrapeFixtureDetails: processing %d fixtures (force=%v)...", len(fixtureIDs), force)

	for _, fid := range fixtureIDs {
		raw, err := s.Client.Get(fmt.Sprintf("football/fixtures/%d", fid), map[string]string{
			"include": "events;lineups.details.type;statistics;scores;participants;referees",
		})
		if err != nil {
			log.Printf("[FootballScraper] Notice: fixture detail %d: %v", fid, err)
			continue
		}

		var env struct {
			Data FixturePayload `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			log.Printf("[FootballScraper] Notice: parse fixture detail %d: %v", fid, err)
			continue
		}
		item := env.Data
		if item.ID == 0 {
			item.ID = fid
		}

		var events []models.FixtureEvent
		var lineups []models.FixtureLineup
		var lineupDetails []models.FixtureLineupDetail
		var scores []models.FixtureScore
		var stats []models.FixtureStatistic

		for _, ev := range item.Events {
			ev.FixtureID = item.ID
			events = append(events, ev)
		}
		for _, lu := range item.Lineups {
			lModel := lu.FixtureLineup
			lModel.FixtureID = item.ID
			lineups = append(lineups, lModel)

			allDetails := append(lu.Details, lu.LineupDetails...)
			allDetails = append(allDetails, lu.LineupDetail...)
			for _, dt := range allDetails {
				dt.FixtureID = item.ID
				dt.LineupID = lModel.ID
				if dt.PlayerID == 0 {
					dt.PlayerID = lModel.PlayerID
				}
				lineupDetails = append(lineupDetails, dt)
			}
		}
		for _, sc := range item.Scores {
			sc.FixtureID = item.ID
			scores = append(scores, sc)
		}
		for _, st := range item.Statistics {
			st.FixtureID = item.ID
			stats = append(stats, st)
		}

		if len(events) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&events, dbBatchSize).Error; err == nil {
				result.Events += len(events)
			} else {
				log.Printf("[FootballScraper] Error saving events fixture %d: %v", fid, err)
			}
		}
		if len(lineups) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&lineups, dbBatchSize).Error; err == nil {
				result.Lineups += len(lineups)
			}
		}
		if len(lineupDetails) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&lineupDetails, dbBatchSize).Error; err == nil {
				result.LineupDetails += len(lineupDetails)
			} else {
				log.Printf("[FootballScraper] Error saving lineup details fixture %d: %v", fid, err)
			}
		}
		if len(scores) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&scores, dbBatchSize).Error; err == nil {
				result.Scores += len(scores)
			}
		}
		if len(stats) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&stats, dbBatchSize).Error; err == nil {
				result.Statistics += len(stats)
			}
		}

		result.FixturesProcessed++
		time.Sleep(350 * time.Millisecond)
	}

	log.Printf("[FootballScraper] ScrapeFixtureDetails done: %d fixtures, %d events, %d lineup_details, %d stats",
		result.FixturesProcessed, result.Events, result.LineupDetails, result.Statistics)
	return result, nil
}

// playerStatItemPayload decodes one player season-statistics row.
type playerStatItemPayload struct {
	SeasonID uint `json:"season_id"`
	TeamID   uint `json:"team_id"`
	Details  []struct {
		TypeID uint            `json:"type_id"`
		Type   *models.Type    `json:"type"`
		Value  json.RawMessage `json:"value"`
	} `json:"details"`
}

// parseStatValue pulls a numeric out of Sportmonks' polymorphic stat `value`
// object ({total:..}, {average:..}, {value:..}) or a bare number.
func parseStatValue(raw json.RawMessage) float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var obj struct {
		Total   *float64 `json:"total"`
		Average *float64 `json:"average"`
		Value   *float64 `json:"value"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if obj.Total != nil {
			return *obj.Total
		}
		if obj.Average != nil {
			return *obj.Average
		}
		if obj.Value != nil {
			return *obj.Value
		}
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	return 0
}

// ScrapePlayerStatistics fetches per-season aggregate stats (goals, assists,
// appearances, minutes, cards, rating) for players in current-season squads that
// don't have stats yet. One request per player, bounded by playerStatsLimit.
// Stat types are matched by name/developer_name so unknown type ids never break.
func (s *FootballScraper) ScrapePlayerStatistics(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) (int, error) {
	seasonIDs := seasonIDsFromMap(currentSeasonIDs...)

	q := s.DB.Model(&models.Squad{}).Distinct("player_id").
		Where("player_id NOT IN (SELECT player_id FROM player_statistics)")
	if len(seasonIDs) > 0 {
		q = q.Where("season_id IN ?", seasonIDs)
	}
	var playerIDs []uint
	q.Limit(playerStatsLimit).Pluck("player_id", &playerIDs)

	if len(playerIDs) == 0 {
		return 0, nil
	}
	log.Printf("[FootballScraper] Scraping season statistics for %d players...", len(playerIDs))

	currentSet := make(map[uint]bool)
	for _, id := range seasonIDs {
		currentSet[id] = true
	}

	total := 0
	for _, pid := range playerIDs {
		raw, err := s.Client.Get(fmt.Sprintf("football/statistics/seasons/players/%d", pid), map[string]string{
			"include": "details.type",
		})
		if err != nil {
			log.Printf("[FootballScraper] Notice: player stats %d: %v", pid, err)
			continue
		}

		var env struct {
			Data []playerStatItemPayload `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			log.Printf("[FootballScraper] Notice: parse player stats %d: %v", pid, err)
			continue
		}

		var stats []models.PlayerStatistic
		for _, item := range env.Data {
			if item.SeasonID == 0 {
				continue
			}
			// Keep only current seasons when we know them
			if len(currentSet) > 0 && !currentSet[item.SeasonID] {
				continue
			}

			ps := models.PlayerStatistic{PlayerID: pid, SeasonID: item.SeasonID, TeamID: item.TeamID}
			for _, d := range item.Details {
				name := ""
				if d.Type != nil {
					name = strings.ToLower(d.Type.DeveloperName + " " + d.Type.Code + " " + d.Type.Name)
				}
				val := parseStatValue(d.Value)
				switch {
				case strings.Contains(name, "assist"):
					ps.Assists = int(val)
				case strings.Contains(name, "goal") && !strings.Contains(name, "conceded") && !strings.Contains(name, "concede"):
					ps.Goals = int(val)
				case strings.Contains(name, "appear"):
					ps.Appearances = int(val)
				case strings.Contains(name, "lineup"):
					ps.Lineups = int(val)
				case strings.Contains(name, "minutes"):
					ps.Minutes = int(val)
				case strings.Contains(name, "yellow"):
					ps.YellowCards = int(val)
				case strings.Contains(name, "red"):
					ps.RedCards = int(val)
				case strings.Contains(name, "rating"):
					r := val
					ps.Rating = &r
				}
			}
			stats = append(stats, ps)
		}

		if len(stats) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&stats, dbBatchSize).Error; err != nil {
				log.Printf("[FootballScraper] Error saving player stats for %d: %v", pid, err)
			} else {
				total += len(stats)
			}
		}

		time.Sleep(350 * time.Millisecond)
	}

	log.Printf("[FootballScraper] Player statistics: saved %d rows", total)
	return total, nil
}

// ScrapePlayerStatisticsInit self-resolves active leagues + current seasons and
// scrapes player statistics — a convenience entry point for initial seeding.
// Bounded by playerStatsLimit per call, but resumable (skips players already
// stored), so call repeatedly until it returns 0.
func (s *FootballScraper) ScrapePlayerStatisticsInit(specificSeasonID uint) (int, error) {
	var activeLeagues []models.League
	if err := s.DB.Where("status = ? AND active = ?", true, true).Find(&activeLeagues).Error; err != nil {
		return 0, fmt.Errorf("failed to query active leagues: %w", err)
	}
	activeLeagueIDs := make(map[uint]bool)
	for _, l := range activeLeagues {
		activeLeagueIDs[l.ID] = true
	}

	var currentSeasons map[uint]bool
	if specificSeasonID > 0 {
		currentSeasons = map[uint]bool{specificSeasonID: true}
	} else {
		currentSeasons = s.GetCurrentSeasonIDs(activeLeagueIDs)
	}
	return s.ScrapePlayerStatistics(activeLeagueIDs, currentSeasons)
}

// seasonIDsFromMap flattens the optional current-season set into a slice.
func seasonIDsFromMap(currentSeasonIDs ...map[uint]bool) []uint {
	var ids []uint
	if len(currentSeasonIDs) > 0 {
		for id := range currentSeasonIDs[0] {
			ids = append(ids, id)
		}
	}
	return ids
}

// ScrapeVenues fetches venues scoped to current/active seasons instead of the
// entire global Sportmonks venue dataset — covers the stadiums that active
// teams actually play in while cutting request volume drastically.
func (s *FootballScraper) ScrapeVenues(currentSeasonIDs ...map[uint]bool) (int, error) {
	seasonIDs := seasonIDsFromMap(currentSeasonIDs...)
	if len(seasonIDs) == 0 {
		log.Println("[FootballScraper] ScrapeVenues: no active seasons resolved, skipping.")
		return 0, nil
	}
	var total int
	for _, sid := range seasonIDs {
		endpoint := fmt.Sprintf("football/venues/seasons/%d", sid)
		count, err := scrapePaginated(s.Client, s.DB, endpoint, nil, func(data []models.Venue) (int, error) {
			err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
			return len(data), err
		})
		if err != nil {
			log.Printf("[FootballScraper] Notice: venues for season %d: %v", sid, err)
		} else {
			total += count
		}
	}
	return total, nil
}

// ScrapeCoaches fetches all coaches.
func (s *FootballScraper) ScrapeCoaches() (int, error) {
	return scrapePaginated(s.Client, s.DB, "football/coaches", nil, func(data []models.Coach) (int, error) {
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
		return len(data), err
	})
}

// ScrapeReferees fetches referees scoped to current/active seasons instead of
// the entire global referee dataset.
func (s *FootballScraper) ScrapeReferees(currentSeasonIDs ...map[uint]bool) (int, error) {
	seasonIDs := seasonIDsFromMap(currentSeasonIDs...)
	if len(seasonIDs) == 0 {
		log.Println("[FootballScraper] ScrapeReferees: no active seasons resolved, skipping.")
		return 0, nil
	}
	var total int
	for _, sid := range seasonIDs {
		endpoint := fmt.Sprintf("football/referees/seasons/%d", sid)
		count, err := scrapePaginated(s.Client, s.DB, endpoint, nil, func(data []models.Referee) (int, error) {
			err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
			return len(data), err
		})
		if err != nil {
			log.Printf("[FootballScraper] Notice: referees for season %d: %v", sid, err)
		} else {
			total += count
		}
	}
	return total, nil
}

// StandingPayload decodes standings with nested details and recent-form.
type StandingPayload struct {
	models.Standing
	Details []models.StandingDetail `json:"details"`
	Form    []models.StandingForm   `json:"form"`
}

// ScrapeStandings fetches standings with nested details and populates standings, standing_details & distinct season_teams pivot table.
func (s *FootballScraper) ScrapeStandings(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) (int, error) {
	var targetSeasons map[uint]bool
	if len(currentSeasonIDs) > 0 {
		targetSeasons = currentSeasonIDs[0]
	}

	extraParams := map[string]string{
		"include": "details;form",
	}
	return scrapePaginated(s.Client, s.DB, "football/standings", extraParams, func(data []StandingPayload) (int, error) {
		var filtered []models.Standing
		var standingDetails []models.StandingDetail
		var standingForms []models.StandingForm
		seenPair := make(map[string]bool)
		var seasonTeams []models.SeasonTeam

		for _, item := range data {
			standing := item.Standing
			if activeLeagueIDs != nil && !activeLeagueIDs[standing.LeagueID] {
				continue
			}
			if targetSeasons != nil && len(targetSeasons) > 0 && !targetSeasons[standing.SeasonID] {
				continue
			}

			filtered = append(filtered, standing)
			for _, dt := range item.Details {
				if dt.StandingID == 0 {
					dt.StandingID = standing.ID
				}
				standingDetails = append(standingDetails, dt)
			}
			for _, fm := range item.Form {
				fm.StandingID = standing.ID // form items inherit their parent standing
				standingForms = append(standingForms, fm)
			}
			if standing.SeasonID > 0 && standing.ParticipantID > 0 {
				key := fmt.Sprintf("%d-%d", standing.SeasonID, standing.ParticipantID)
				if !seenPair[key] {
					seenPair[key] = true
					seasonTeams = append(seasonTeams, models.SeasonTeam{
						SeasonID: standing.SeasonID,
						TeamID:   standing.ParticipantID,
					})
				}
			}
		}
		if len(filtered) == 0 {
			return 0, nil
		}
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&filtered, dbBatchSize).Error
		if len(standingDetails) > 0 {
			_ = s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&standingDetails, dbBatchSize)
		}
		if len(standingForms) > 0 {
			_ = s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&standingForms, dbBatchSize)
		}
		if len(seasonTeams) > 0 {
			_ = s.DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&seasonTeams, dbBatchSize)
		}
		return len(filtered), err
	})
}

// ScrapeTopscorers fetches topscorers for current seasons belonging to active leagues.
func (s *FootballScraper) ScrapeTopscorers(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) (int, error) {
	var targetSeasons map[uint]bool
	if len(currentSeasonIDs) > 0 {
		targetSeasons = currentSeasonIDs[0]
	} else {
		targetSeasons = s.GetCurrentSeasonIDs(activeLeagueIDs)
	}

	var seasonIDs []uint
	for id := range targetSeasons {
		seasonIDs = append(seasonIDs, id)
	}

	var seasons []models.Season
	query := s.DB
	if len(seasonIDs) > 0 {
		query = query.Where("id IN ?", seasonIDs)
	} else if len(activeLeagueIDs) > 0 {
		var ids []uint
		for id := range activeLeagueIDs {
			ids = append(ids, id)
		}
		query = query.Where("league_id IN ?", ids)
	}
	if err := query.Find(&seasons).Error; err != nil {
		return 0, fmt.Errorf("failed to query seasons for topscorers: %w", err)
	}

	var total int
	for _, season := range seasons {
		currentSeasonID := season.ID
		currentLeagueID := season.LeagueID
		endpoint := fmt.Sprintf("football/topscorers/seasons/%d", season.ID)
		count, err := scrapePaginated(s.Client, s.DB, endpoint, nil, func(data []models.Topscorer) (int, error) {
			for i := range data {
				data[i].SeasonID = &currentSeasonID
				data[i].LeagueID = &currentLeagueID
			}
			err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
			return len(data), err
		})
		if err != nil {
			log.Printf("[FootballScraper] Notice: topscorers for season %d: %v", season.ID, err)
		} else {
			total += count
		}
	}

	// Backfill any previously scraped topscorers that had null season_id or league_id
	_ = s.DB.Exec("UPDATE topscorers SET season_id = (SELECT stages.season_id FROM stages WHERE stages.id = topscorers.stage_id), league_id = (SELECT stages.league_id FROM stages WHERE stages.id = topscorers.stage_id) WHERE (season_id IS NULL OR league_id IS NULL) AND stage_id IS NOT NULL").Error

	return total, nil
}

// ScrapeRivals fetches team rivals and populates team_rivals pivot table.
func (s *FootballScraper) ScrapeRivals() (int, error) {
	return scrapePaginated(s.Client, s.DB, "football/rivals", nil, func(data []models.TeamRival) (int, error) {
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
		return len(data), err
	})
}

// ScrapeTransfers fetches transfers scoped to the teams in active leagues /
// current seasons (football/transfers/teams/:id) instead of the entire global
// transfer feed — the global endpoint spans every transfer worldwide and burns
// quota on data the portal never shows.
func (s *FootballScraper) ScrapeTransfers(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) (int, error) {
	pairs := s.getActiveSeasonTeams(activeLeagueIDs, currentSeasonIDs...)

	// Distinct team ids across active seasons
	teamSet := make(map[uint]bool)
	for _, p := range pairs {
		if p.TeamID > 0 {
			teamSet[p.TeamID] = true
		}
	}
	if len(teamSet) == 0 {
		log.Println("[FootballScraper] ScrapeTransfers: no active teams resolved, skipping.")
		return 0, nil
	}

	var total int
	for teamID := range teamSet {
		endpoint := fmt.Sprintf("football/transfers/teams/%d", teamID)
		count, err := scrapePaginated(s.Client, s.DB, endpoint, nil, func(data []models.Transfer) (int, error) {
			err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
			return len(data), err
		})
		if err != nil {
			log.Printf("[FootballScraper] Notice: transfers for team %d: %v", teamID, err)
		} else {
			total += count
		}
	}
	return total, nil
}

// ScrapeStates fetches all match states (NS, 1H, HT, 2H, FT, ET, PEN, etc.).
func (s *FootballScraper) ScrapeStates() (int, error) {
	return scrapePaginated(s.Client, s.DB, "football/states", nil, func(data []models.State) (int, error) {
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
		return len(data), err
	})
}

// scrapePaginated fetches all pages from a Sportmonks GET ALL endpoint and saves each page via saveFunc.
func scrapePaginated[T any](client *SportmonksClient, db *gorm.DB, endpoint string, extraParams map[string]string, saveFunc func(data []T) (int, error)) (int, error) {
	var total int
	page := 1
	var cursor string

	for {
		params := make(map[string]string)
		for k, v := range extraParams {
			params[k] = v
		}

		if cursor != "" {
			// When cursor is used, Sportmonks strictly forbids "per_page" and "page"
			params["cursor"] = cursor
		} else {
			params["page"] = strconv.Itoa(page)
			params["per_page"] = strconv.Itoa(perPage)
		}

		raw, err := client.Get(endpoint, params)
		if err != nil {
			return total, fmt.Errorf("failed to fetch %s (page %d, cursor %s): %w", endpoint, page, cursor, err)
		}

		var envelope FootballResponseEnvelope[T]
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return total, fmt.Errorf("failed to parse %s json: %w", endpoint, err)
		}

		if len(envelope.Data) > 0 {
			savedCount, err := saveFunc(envelope.Data)
			if err != nil {
				return total, fmt.Errorf("failed to save %s page %d: %w", endpoint, page, err)
			}
			total += savedCount
		}

		if envelope.Pagination == nil || !envelope.Pagination.HasMore {
			break
		}

		// Extract next cursor token from next_cursor or next_page
		nextCursor := ""
		if envelope.Pagination.NextCursor != nil && *envelope.Pagination.NextCursor != "" {
			nextCursor = extractCursor(*envelope.Pagination.NextCursor)
		} else if envelope.Pagination.NextPage != nil && *envelope.Pagination.NextPage != "" {
			nextCursor = extractCursor(*envelope.Pagination.NextPage)
		}

		if nextCursor != "" {
			cursor = nextCursor
		} else {
			page++
		}

		// Rate limit: ~3 req/sec = 180 req/min (Sportmonks standard plan limit)
		time.Sleep(350 * time.Millisecond)
	}

	log.Printf("[Scraper] %s: fetched %d records", endpoint, total)
	return total, nil
}
