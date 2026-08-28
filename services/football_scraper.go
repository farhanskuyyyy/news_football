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
	return scrapePaginated(s.Client, s.DB, "football/leagues", nil, func(data []models.League) (int, error) {
		for i := range data {
			data[i].Status = true
		}
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&data, dbBatchSize).Error
		return len(data), err
	})
}

// ScrapeAllFootball orchestrates scraping across all football sub-entities and pivot tables.
// You can easily comment/uncomment any line below to skip or run specific datasets.
func (s *FootballScraper) ScrapeAllFootball() (*FootballScrapeResult, error) {
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
	log.Printf("[FootballScraper] Scraping data for %d active leagues", len(activeLeagues))

	var err error

	// =========================================================================
	// TINGGAL COMMENT / UNCOMMENT BAGIAN DI BAWAH INI UNTUK SKIP ATAU RUN
	// =========================================================================

	// 1. Seasons
	if result.Seasons, err = s.ScrapeSeasons(activeLeagueIDs); err != nil {
		log.Printf("[FootballScraper] Error scraping seasons: %v", err)
	}

	// 2. Stages
	if result.Stages, err = s.ScrapeStages(activeLeagueIDs); err != nil {
		log.Printf("[FootballScraper] Error scraping stages: %v", err)
	}

	// 3. Rounds
	if result.Rounds, err = s.ScrapeRounds(activeLeagueIDs); err != nil {
		log.Printf("[FootballScraper] Error scraping rounds: %v", err)
	}

	// 4. Teams (Hanya fetch klub dari musim/liga yang aktif)
	if result.Teams, err = s.ScrapeTeams(activeLeagueIDs); err != nil {
		log.Printf("[FootballScraper] Error scraping teams: %v", err)
	}

	// 5. Standings (Populates standings & distinct season_teams pivot)
	if result.Standings, err = s.ScrapeStandings(activeLeagueIDs); err != nil {
		log.Printf("[FootballScraper] Error scraping standings: %v", err)
	}

	// 6. Squads & Players (Hanya fetch skuad & pemain spesifik per (season_id, team_id))
	if result.Squads, err = s.ScrapeSquads(activeLeagueIDs); err != nil {
		log.Printf("[FootballScraper] Error scraping squads: %v", err)
	}

	// 7. Players (Extended profile per klub aktif)
	if result.Players, err = s.ScrapePlayers(activeLeagueIDs); err != nil {
		log.Printf("[FootballScraper] Error scraping players: %v", err)
	}

	// 8. Fixtures (Populates fixtures, events, lineups, statistics, scores & fixture_referees)
	if result.Fixtures, err = s.ScrapeFixtures(activeLeagueIDs); err != nil {
		log.Printf("[FootballScraper] Error scraping fixtures: %v", err)
	}

	// 9. Venues
	if result.Venues, err = s.ScrapeVenues(); err != nil {
		log.Printf("[FootballScraper] Error scraping venues: %v", err)
	}

	// 10. Coaches
	if result.Coaches, err = s.ScrapeCoaches(); err != nil {
		log.Printf("[FootballScraper] Error scraping coaches: %v", err)
	}

	// 11. Referees
	if result.Referees, err = s.ScrapeReferees(); err != nil {
		log.Printf("[FootballScraper] Error scraping referees: %v", err)
	}

	// 12. Topscorers
	if result.Topscorers, err = s.ScrapeTopscorers(activeLeagueIDs); err != nil {
		log.Printf("[FootballScraper] Error scraping topscorers: %v", err)
	}

	// 13. Rivals (Populates team_rivals pivot table)
	if result.Rivals, err = s.ScrapeRivals(); err != nil {
		log.Printf("[FootballScraper] Error scraping rivals: %v", err)
	}

	// 14. Transfers
	if result.Transfers, err = s.ScrapeTransfers(); err != nil {
		log.Printf("[FootballScraper] Error scraping transfers: %v", err)
	}

	// =========================================================================

	return result, nil
}

// -----------------------------------------------------------------------------
// Individual Scraping Methods
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

// ScrapeStages fetches stages and filters them by active leagues.
func (s *FootballScraper) ScrapeStages(activeLeagueIDs map[uint]bool) (int, error) {
	return scrapePaginated(s.Client, s.DB, "football/stages", nil, func(data []models.Stage) (int, error) {
		var filtered []models.Stage
		for _, stage := range data {
			if activeLeagueIDs == nil || activeLeagueIDs[stage.LeagueID] {
				filtered = append(filtered, stage)
			}
		}
		if len(filtered) == 0 {
			return 0, nil
		}
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&filtered, dbBatchSize).Error
		return len(filtered), err
	})
}

// ScrapeRounds fetches rounds and filters them by active leagues.
func (s *FootballScraper) ScrapeRounds(activeLeagueIDs map[uint]bool) (int, error) {
	return scrapePaginated(s.Client, s.DB, "football/rounds", nil, func(data []models.Round) (int, error) {
		var filtered []models.Round
		for _, round := range data {
			if activeLeagueIDs == nil || activeLeagueIDs[round.LeagueID] {
				filtered = append(filtered, round)
			}
		}
		if len(filtered) == 0 {
			return 0, nil
		}
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&filtered, dbBatchSize).Error
		return len(filtered), err
	})
}

// ScrapeTeams fetches teams only for seasons belonging to active leagues (via football/teams/seasons/:seasonId).
func (s *FootballScraper) ScrapeTeams(activeLeagueIDs map[uint]bool) (int, error) {
	var seasons []models.Season
	query := s.DB
	if len(activeLeagueIDs) > 0 {
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

	log.Printf("[FootballScraper] Scraping teams for %d active seasons...", len(seasons))

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

// getActiveSeasonTeams retrieves distinct (season_id, team_id) pairs for the active leagues.
func (s *FootballScraper) getActiveSeasonTeams(activeLeagueIDs map[uint]bool) []models.SeasonTeam {
	var activeSeasonIDs []uint
	if len(activeLeagueIDs) > 0 {
		var leagueIDs []uint
		for id := range activeLeagueIDs {
			leagueIDs = append(leagueIDs, id)
		}
		s.DB.Model(&models.Season{}).Where("league_id IN ?", leagueIDs).Pluck("id", &activeSeasonIDs)
	}

	var rawPairs []models.SeasonTeam

	// 1. Check SeasonTeam table
	query := s.DB.Model(&models.SeasonTeam{})
	if len(activeSeasonIDs) > 0 {
		query = query.Where("season_id IN ?", activeSeasonIDs)
	}
	query.Select("DISTINCT season_id, team_id").Find(&rawPairs)

	// 2. Check Standing table for distinct (season_id, participant_id)
	if len(rawPairs) == 0 {
		var standings []models.Standing
		stQuery := s.DB.Model(&models.Standing{})
		if len(activeSeasonIDs) > 0 {
			stQuery = stQuery.Where("season_id IN ?", activeSeasonIDs)
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
	if len(rawPairs) == 0 && len(activeSeasonIDs) > 0 {
		var teamIDs []uint
		s.DB.Model(&models.Team{}).Pluck("id", &teamIDs)
		for _, snID := range activeSeasonIDs {
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
func (s *FootballScraper) ScrapeSquads(activeLeagueIDs map[uint]bool) (int, error) {
	seasonTeams := s.getActiveSeasonTeams(activeLeagueIDs)
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
func (s *FootballScraper) ScrapePlayers(activeLeagueIDs map[uint]bool) (int, error) {
	seasonTeams := s.getActiveSeasonTeams(activeLeagueIDs)
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

// FixturePayload decodes a fixture along with its nested includes.
type FixturePayload struct {
	models.Fixture
	Events     []models.FixtureEvent     `json:"events"`
	Lineups    []models.FixtureLineup    `json:"lineups"`
	Scores     []models.FixtureScore     `json:"scores"`
	Statistics []models.FixtureStatistic `json:"statistics"`
	Referees   []struct {
		ID        uint  `json:"id"`
		RefereeID uint  `json:"referee_id"`
		TypeID    *uint `json:"type_id"`
	} `json:"referees"`
	Participants []struct {
		ID uint `json:"id"`
	} `json:"participants"`
}

// ScrapeFixtures fetches fixtures and nested sub-entities (events, lineups, statistics, scores, referees).
func (s *FootballScraper) ScrapeFixtures(activeLeagueIDs map[uint]bool) (int, error) {
	extraParams := map[string]string{
		"include": "events;lineups;scores;statistics;referees;participants",
	}

	return scrapePaginated(s.Client, s.DB, "football/fixtures", extraParams, func(data []FixturePayload) (int, error) {
		var fixtures []models.Fixture
		var events []models.FixtureEvent
		var lineups []models.FixtureLineup
		var scores []models.FixtureScore
		var stats []models.FixtureStatistic
		var referees []models.FixtureReferee
		var seasonTeams []models.SeasonTeam

		for _, item := range data {
			if activeLeagueIDs != nil && !activeLeagueIDs[item.LeagueID] {
				continue
			}

			fixtures = append(fixtures, item.Fixture)

			for _, ev := range item.Events {
				ev.FixtureID = item.ID
				events = append(events, ev)
			}
			for _, lu := range item.Lineups {
				lu.FixtureID = item.ID
				lineups = append(lineups, lu)
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

// ScrapeStandings fetches standings and populates standings & distinct season_teams pivot table.
func (s *FootballScraper) ScrapeStandings(activeLeagueIDs map[uint]bool) (int, error) {
	return scrapePaginated(s.Client, s.DB, "football/standings", nil, func(data []models.Standing) (int, error) {
		var filtered []models.Standing
		seenPair := make(map[string]bool)
		var seasonTeams []models.SeasonTeam

		for _, standing := range data {
			if activeLeagueIDs == nil || activeLeagueIDs[standing.LeagueID] {
				filtered = append(filtered, standing)
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
		}
		if len(filtered) == 0 {
			return 0, nil
		}
		err := s.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&filtered, dbBatchSize).Error
		if len(seasonTeams) > 0 {
			_ = s.DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&seasonTeams, dbBatchSize)
		}
		return len(filtered), err
	})
}

// ScrapeTopscorers fetches topscorers for all seasons belonging to the active leagues.
func (s *FootballScraper) ScrapeTopscorers(activeLeagueIDs map[uint]bool) (int, error) {
	var seasons []models.Season
	query := s.DB
	if len(activeLeagueIDs) > 0 {
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
