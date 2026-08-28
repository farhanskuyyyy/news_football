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

// dbBatchSize is the number of records to upsert per DB transaction.
const dbBatchSize = 200

// Dynamic TTL Constants for Table Synchronization (saving Sportmonks rate limits & quota)
const (
	TTLFixtures   = 1 * time.Hour
	TTLStandings  = 1 * time.Hour
	TTLTopscorers = 1 * time.Hour
	TTLTransfers  = 12 * time.Hour
	TTLSquads     = 24 * time.Hour
	TTLPlayers    = 24 * time.Hour
	TTLRounds     = 24 * time.Hour
	TTLStages     = 24 * time.Hour
	TTLTeams      = 7 * 24 * time.Hour
	TTLSeasons    = 7 * 24 * time.Hour
	TTLVenues     = 30 * 24 * time.Hour
	TTLCoaches    = 30 * 24 * time.Hour
	TTLReferees   = 30 * 24 * time.Hour
	TTLRivals     = 30 * 24 * time.Hour
	TTLStates     = 30 * 24 * time.Hour
	TTLLeagues    = 30 * 24 * time.Hour
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

	// 8. Fixtures (TTL: 1h, filtered by current seasons)
	if s.ShouldSync("fixtures", TTLFixtures, isForce) {
		result.Fixtures, err = s.ScrapeFixtures(activeLeagueIDs, currentSeasonIDs)
		s.MarkSynced("fixtures", result.Fixtures, TTLFixtures, err)
		if err != nil {
			log.Printf("[FootballScraper] Error scraping fixtures: %v", err)
		}
	}

	// 9. Venues (TTL: 30 days)
	if s.ShouldSync("venues", TTLVenues, isForce) {
		result.Venues, err = s.ScrapeVenues()
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

	// 11. Referees (TTL: 30 days)
	if s.ShouldSync("referees", TTLReferees, isForce) {
		result.Referees, err = s.ScrapeReferees()
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

	// 14. Transfers (TTL: 12h)
	if s.ShouldSync("transfers", TTLTransfers, isForce) {
		result.Transfers, err = s.ScrapeTransfers()
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

	var total int
	for _, season := range seasons {
		currentSeasonID := season.ID
		endpoint := fmt.Sprintf("football/teams/seasons/%d", season.ID)
		count, err := scrapePaginated(s.Client, s.DB, endpoint, nil, func(data []models.Team) (int, error) {
			if err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error; err != nil {
				return 0, err
			}

			// Automatically populate SeasonTeam pivot table
			var seasonTeams []models.SeasonTeam
			for _, team := range data {
				seasonTeams = append(seasonTeams, models.SeasonTeam{
					SeasonID: currentSeasonID,
					TeamID:   team.ID,
				})
			}
			if len(seasonTeams) > 0 {
				_ = s.DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&seasonTeams, dbBatchSize)
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
	Events       []models.FixtureEvent      `json:"events"`
	Lineups      []LineupWithDetailsPayload `json:"lineups"`
	Scores       []models.FixtureScore      `json:"scores"`
	Statistics   []models.FixtureStatistic  `json:"statistics"`
	Referees     []struct {
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
			_ = s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&lineupDetails, dbBatchSize)
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

// ScrapeVenues fetches all venues.
func (s *FootballScraper) ScrapeVenues() (int, error) {
	return scrapePaginated(s.Client, s.DB, "football/venues", nil, func(data []models.Venue) (int, error) {
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
		return len(data), err
	})
}

// ScrapeCoaches fetches all coaches.
func (s *FootballScraper) ScrapeCoaches() (int, error) {
	return scrapePaginated(s.Client, s.DB, "football/coaches", nil, func(data []models.Coach) (int, error) {
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
		return len(data), err
	})
}

// ScrapeReferees fetches all referees.
func (s *FootballScraper) ScrapeReferees() (int, error) {
	return scrapePaginated(s.Client, s.DB, "football/referees", nil, func(data []models.Referee) (int, error) {
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
		return len(data), err
	})
}

// StandingPayload decodes standings with nested details.
type StandingPayload struct {
	models.Standing
	Details []models.StandingDetail `json:"details"`
}

// ScrapeStandings fetches standings with nested details and populates standings, standing_details & distinct season_teams pivot table.
func (s *FootballScraper) ScrapeStandings(activeLeagueIDs map[uint]bool, currentSeasonIDs ...map[uint]bool) (int, error) {
	var targetSeasons map[uint]bool
	if len(currentSeasonIDs) > 0 {
		targetSeasons = currentSeasonIDs[0]
	}

	extraParams := map[string]string{
		"include": "details",
	}
	return scrapePaginated(s.Client, s.DB, "football/standings", extraParams, func(data []StandingPayload) (int, error) {
		var filtered []models.Standing
		var standingDetails []models.StandingDetail
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

// ScrapeTransfers fetches all transfers.
func (s *FootballScraper) ScrapeTransfers() (int, error) {
	return scrapePaginated(s.Client, s.DB, "football/transfers", nil, func(data []models.Transfer) (int, error) {
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
		return len(data), err
	})
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
