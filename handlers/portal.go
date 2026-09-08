package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/farhanarfianto/apigo-docker/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type PortalHandler struct {
	DB *gorm.DB
}

func NewPortalHandler(db *gorm.DB) *PortalHandler {
	return &PortalHandler{DB: db}
}

// goalTopscorerTypeID is the Sportmonks type id for the "Goal Topscorer"
// ranking, used as the default topscorer metric.
const goalTopscorerTypeID = uint(208)

// ─────────────────────────────────────────────────────────────────────────────
// 1. LEAGUES & SEASONS
// ─────────────────────────────────────────────────────────────────────────────

// LeagueListItem represents a league with season summary info.
type LeagueListItem struct {
	models.League
	SeasonsCount int64 `json:"seasons_count"`
}

// GetLeagues returns all leagues from DB, ordered by active and status.
// UpdateLeagueStatus enables/disables a league for scraping (the `status` flag).
// Accepts ?status=true|false or a JSON body {"status":bool}.
func (h *PortalHandler) UpdateLeagueStatus(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid league id")
	}

	// Accept status from query, form, or JSON body (robust across content types).
	var status bool
	if q := c.QueryParam("status"); q != "" {
		status = q == "true" || q == "1"
	} else if f := c.FormValue("status"); f != "" {
		status = f == "true" || f == "1"
	} else {
		var body struct {
			Status bool `json:"status" form:"status"`
		}
		_ = c.Bind(&body)
		status = body.Status
	}

	res := h.DB.Model(&models.League{}).Where("id = ?", id).Update("status", status)
	if res.Error != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "league not found")
	}

	return c.JSON(http.StatusOK, echo.Map{
		"status":    "success",
		"league_id": id,
		"enabled":   status,
	})
}

func (h *PortalHandler) GetLeagues(c echo.Context) error {
	var leagues []models.League
	query := h.DB.Order("status DESC, active DESC, name ASC")

	// The admin `status` flag is the sole gate for public visibility; the
	// Sportmonks `active` column is informational only.
	if c.QueryParam("active_only") == "true" {
		query = query.Where("status = ?", true)
	}

	if err := query.Find(&leagues).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	var items []LeagueListItem
	for _, l := range leagues {
		var cnt int64
		h.DB.Model(&models.Season{}).Where("league_id = ?", l.ID).Count(&cnt)
		items = append(items, LeagueListItem{
			League:       l,
			SeasonsCount: cnt,
		})
	}

	return c.JSON(http.StatusOK, echo.Map{
		"status": "success",
		"data":   items,
	})
}

// GetLeagueSeasons returns all seasons for a specific league.
func (h *PortalHandler) GetLeagueSeasons(c echo.Context) error {
	leagueID, err := strconv.Atoi(c.Param("league_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid league id")
	}

	var seasons []models.Season
	if err := h.DB.Where("league_id = ?", leagueID).
		Order("is_current DESC, id DESC").
		Find(&seasons).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	var league models.League
	h.DB.First(&league, leagueID)

	return c.JSON(http.StatusOK, echo.Map{
		"status": "success",
		"league": league,
		"data":   seasons,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. SEASON OVERVIEW & STANDINGS
// ─────────────────────────────────────────────────────────────────────────────

// GetSeasonOverview returns high-level season metrics (totals).
func (h *PortalHandler) GetSeasonOverview(c echo.Context) error {
	seasonID, err := strconv.Atoi(c.Param("season_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid season id")
	}

	var season models.Season
	if err := h.DB.First(&season, seasonID).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "season not found")
	}

	var league models.League
	h.DB.First(&league, season.LeagueID)

	var totalTeams int64
	h.DB.Model(&models.SeasonTeam{}).Where("season_id = ?", seasonID).Count(&totalTeams)

	var totalFixtures int64
	h.DB.Model(&models.Fixture{}).Where("season_id = ?", seasonID).Count(&totalFixtures)

	var totalRounds int64
	h.DB.Model(&models.Round{}).Where("season_id = ?", seasonID).Count(&totalRounds)

	var totalStages int64
	h.DB.Model(&models.Stage{}).Where("season_id = ?", seasonID).Count(&totalStages)

	// A season is a cup when it is played in ties somewhere — qualifying or
	// knock-out stages. That, not the presence of a group stage, is what makes
	// the bracket view meaningful: every league has exactly one group stage.
	var bracketStages int64
	h.DB.Model(&models.Stage{}).
		Where("season_id = ? AND type_id IN ?", seasonID, []uint{knockOutStageTypeID, qualifyingStageTypeID}).
		Count(&bracketStages)

	return c.JSON(http.StatusOK, echo.Map{
		"status": "success",
		"data": echo.Map{
			"season":         season,
			"league":         league,
			"total_teams":    totalTeams,
			"total_fixtures": totalFixtures,
			"total_rounds":   totalRounds,
			"total_stages":   totalStages,
			"has_bracket":    bracketStages > 0,
		},
	})
}

// EnrichedStandingDetail represents a single standing metric joined with its Type name.
type EnrichedStandingDetail struct {
	ID           uint         `json:"id"`
	StandingID   uint         `json:"standing_id"`
	StandingType string       `json:"standing_type"`
	TypeID       uint         `json:"type_id"`
	Value        int          `json:"value"`
	TypeName     string       `json:"type_name"`
	Type         *models.Type `json:"type,omitempty"`
}

// StandingItem represents a standing row enriched with team info, detailed metrics (P, W, D, L, GF, GA, GD, PTS), and joined details.
type StandingItem struct {
	models.Standing
	Team           *models.Team             `json:"team,omitempty"`
	Played         int                      `json:"played"`
	Won            int                      `json:"won"`
	Draw           int                      `json:"draw"`
	Lost           int                      `json:"lost"`
	GoalsFor       int                      `json:"goals_for"`
	GoalsAgainst   int                      `json:"goals_against"`
	GoalDifference int                      `json:"goal_difference"`
	Form           []string                 `json:"form"` // recent results, oldest→newest e.g. ["W","W","D","L","W"]
	Details        []EnrichedStandingDetail `json:"details"`
}

// GetSeasonStandings returns standings for a specific season, enriched with team data and standing.details joined to types.
func (h *PortalHandler) GetSeasonStandings(c echo.Context) error {
	seasonID, err := strconv.Atoi(c.Param("season_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid season id")
	}

	var standings []models.Standing
	if err := h.DB.Where("season_id = ?", seasonID).
		Order("position ASC").
		Find(&standings).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, echo.Map{
		"status": "success",
		"data":   h.buildStandingItems(standings),
	})
}

// buildStandingItems enriches raw standing rows with team, per-metric details
// (P/W/D/L/GF/GA/GD) and recent form. Shared by the standings tab and the
// cup bracket, which renders the same table for its group/league stages.
func (h *PortalHandler) buildStandingItems(standings []models.Standing) []StandingItem {
	var standingIDs []uint
	for _, st := range standings {
		standingIDs = append(standingIDs, st.ID)
	}

	// Fetch all standing_details for these standings
	var allDetails []models.StandingDetail
	if len(standingIDs) > 0 {
		h.DB.Where("standing_id IN ?", standingIDs).Find(&allDetails)
	}

	// Fetch recent-form rows and group as ordered W/D/L letters per standing
	formByStanding := make(map[uint][]string)
	if len(standingIDs) > 0 {
		var allForms []models.StandingForm
		h.DB.Where("standing_id IN ?", standingIDs).Order("sort_order ASC").Find(&allForms)
		for _, fm := range allForms {
			if fm.Form == "" {
				continue
			}
			formByStanding[fm.StandingID] = append(formByStanding[fm.StandingID], strings.ToUpper(fm.Form))
		}
	}

	// Fetch distinct Types
	var typeIDs []uint
	for _, dt := range allDetails {
		if dt.TypeID > 0 {
			typeIDs = append(typeIDs, dt.TypeID)
		}
	}
	typeMap := make(map[uint]models.Type)
	if len(typeIDs) > 0 {
		var types []models.Type
		h.DB.Where("id IN ?", typeIDs).Find(&types)
		for _, t := range types {
			typeMap[t.ID] = t
		}
	}

	// Group details by standing_id
	detailsByStanding := make(map[uint][]EnrichedStandingDetail)
	for _, dt := range allDetails {
		tp, hasType := typeMap[dt.TypeID]
		tName := ""
		if hasType {
			tName = tp.Name
			if tName == "" {
				tName = tp.DeveloperName
			}
		}
		if tName == "" {
			tName = fmt.Sprintf("Type #%d", dt.TypeID)
		}

		var tpPtr *models.Type
		if hasType {
			tpPtr = &tp
		}

		detailsByStanding[dt.StandingID] = append(detailsByStanding[dt.StandingID], EnrichedStandingDetail{
			ID:           dt.ID,
			StandingID:   dt.StandingID,
			StandingType: dt.StandingType,
			TypeID:       dt.TypeID,
			Value:        dt.Value,
			TypeName:     tName,
			Type:         tpPtr,
		})
	}

	items := make([]StandingItem, 0, len(standings))
	for _, st := range standings {
		var tm models.Team
		if st.ParticipantID > 0 {
			h.DB.First(&tm, st.ParticipantID)
		}

		details := detailsByStanding[st.ID]
		if details == nil {
			details = []EnrichedStandingDetail{}
		}

		played := 0
		won := 0
		draw := 0
		lost := 0
		gf := 0
		ga := 0
		gd := 0

		for _, dt := range details {
			// Skip HOME/AWAY split rows — they reuse the same metric names
			// ("won", "lost", ...) as the overall row and would otherwise
			// overwrite the totals depending on array order (the cause of a few
			// teams showing wrong W/D/L). Only the overall row is aggregated.
			stLower := strings.ToLower(dt.StandingType)
			if strings.Contains(stLower, "home") || strings.Contains(stLower, "away") {
				continue
			}

			tLower := strings.ToLower(dt.TypeName)
			codeLower := ""
			if dt.Type != nil {
				codeLower = strings.ToLower(dt.Type.Code + " " + dt.Type.DeveloperName)
			}
			combined := tLower + " " + codeLower

			if strings.Contains(combined, "home") || strings.Contains(combined, "away") {
				continue
			}

			if strings.Contains(combined, "played") || strings.Contains(combined, "matches-played") || dt.TypeID == 129 {
				played = dt.Value
			} else if strings.Contains(combined, "won") || strings.Contains(combined, "win") || dt.TypeID == 130 {
				won = dt.Value
			} else if strings.Contains(combined, "draw") || dt.TypeID == 131 {
				draw = dt.Value
			} else if strings.Contains(combined, "lost") || strings.Contains(combined, "defeat") || dt.TypeID == 132 {
				lost = dt.Value
			} else if strings.Contains(combined, "goals-for") || strings.Contains(combined, "goal-for") || strings.Contains(combined, "goals-scored") || strings.Contains(combined, "scored") || dt.TypeID == 133 {
				gf = dt.Value
			} else if strings.Contains(combined, "goals-against") || strings.Contains(combined, "goal-against") || strings.Contains(combined, "conceded") || dt.TypeID == 134 {
				ga = dt.Value
			} else if strings.Contains(combined, "difference") || strings.Contains(combined, "goal-difference") || dt.TypeID == 135 {
				gd = dt.Value
			}
		}

		if gd == 0 && (gf > 0 || ga > 0) {
			gd = gf - ga
		}
		if played == 0 && (won > 0 || draw > 0 || lost > 0) {
			played = won + draw + lost
		}

		// Recent form: keep last 5 (most recent) results, oldest→newest
		form := formByStanding[st.ID]
		if len(form) > 5 {
			form = form[len(form)-5:]
		}
		if form == nil {
			form = []string{}
		}

		items = append(items, StandingItem{
			Standing:       st,
			Team:           &tm,
			Played:         played,
			Won:            won,
			Draw:           draw,
			Lost:           lost,
			GoalsFor:       gf,
			GoalsAgainst:   ga,
			GoalDifference: gd,
			Form:           form,
			Details:        details,
		})
	}

	return items
}

// ─────────────────────────────────────────────────────────────────────────────
// 2b. CUP BRACKET (STAGES)
// ─────────────────────────────────────────────────────────────────────────────

// Sportmonks stage type ids. A season counts as a cup when it has at least one
// qualifying or knock-out stage: every league — domestic ones included — has a
// single GROUP_STAGE, so that type alone cannot tell a cup from a league.
const (
	groupStageTypeID      = uint(223)
	knockOutStageTypeID   = uint(224)
	qualifyingStageTypeID = uint(225)
)

// isBracketStageType reports whether a stage is played as ties (one or two
// legs) rather than as a table.
func isBracketStageType(typeID *uint) bool {
	return typeID != nil && (*typeID == knockOutStageTypeID || *typeID == qualifyingStageTypeID)
}

// BracketLeg is one fixture of a tie.
type BracketLeg struct {
	FixtureID  uint          `json:"fixture_id"`
	Name       string        `json:"name"`
	StartingAt *string       `json:"starting_at"`
	StateID    *uint         `json:"state_id"`
	State      *models.State `json:"state,omitempty"`
	HomeTeamID uint          `json:"home_team_id"`
	AwayTeamID uint          `json:"away_team_id"`
	HomeGoals  *int          `json:"home_goals"`
	AwayGoals  *int          `json:"away_goals"`
	ResultInfo *string       `json:"result_info"`
}

// BracketSide is one team in a tie, with its aggregate over the legs played.
type BracketSide struct {
	Team      *models.Team `json:"team,omitempty"`
	TeamID    uint         `json:"team_id"`
	Aggregate int          `json:"aggregate"`
}

// BracketTie pairs the legs two teams played against each other inside one stage.
type BracketTie struct {
	// ID is the lowest fixture id of the tie — stable, and enough for the UI to
	// wire a tie to the one it feeds.
	ID        uint          `json:"id"`
	NextTieID *uint         `json:"next_tie_id"`
	Sides     []BracketSide `json:"sides"`
	Legs      []BracketLeg  `json:"legs"`
	// WinnerTeamID is set only when the aggregate decides the tie. A level
	// aggregate leaves it nil — the shootout/extra-time outcome is not stored
	// as data, only described in the last leg's ResultInfo.
	WinnerTeamID *uint   `json:"winner_team_id"`
	DecidedBy    string  `json:"decided_by"` // "aggregate" | "level" | "pending"
	ResultInfo   *string `json:"result_info"`
	Played       bool    `json:"played"`
}

// BracketStage is one stage of a season, rendered either as ties or as a table.
type BracketStage struct {
	models.Stage
	TypeName string `json:"type_name"`
	Kind     string `json:"kind"` // "bracket" | "table"
	// IsKnockout marks the stages that form the elimination tree. Qualifying
	// rounds are ties too, but new entrants join each round, so they do not
	// chain into a bracket and are listed rather than drawn.
	IsKnockout bool           `json:"is_knockout"`
	Ties       []BracketTie   `json:"ties"`
	Standings  []StandingItem `json:"standings"`
}

// GetSeasonBracket returns a season's stages in playing order: knock-out and
// qualifying stages as ties (legs paired by the two teams involved, with
// aggregate scores), every other stage as its standings table.
func (h *PortalHandler) GetSeasonBracket(c echo.Context) error {
	seasonID, err := strconv.Atoi(c.Param("season_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid season id")
	}

	var stages []models.Stage
	if err := h.DB.Where("season_id = ?", seasonID).
		Order("sort_order ASC, id ASC").
		Find(&stages).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	typeNames := h.stageTypeNames(stages)
	teamCache := make(map[uint]*models.Team)

	items := make([]BracketStage, 0, len(stages))
	for _, st := range stages {
		item := BracketStage{
			Stage:     st,
			Ties:      []BracketTie{},
			Standings: []StandingItem{},
			Kind:      "table",
		}
		if st.TypeID != nil {
			item.TypeName = typeNames[*st.TypeID]
		}

		if isBracketStageType(st.TypeID) {
			item.Kind = "bracket"
			item.IsKnockout = st.TypeID != nil && *st.TypeID == knockOutStageTypeID
			ties, err := h.buildStageTies(st.ID, teamCache)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			item.Ties = ties
		} else {
			var standings []models.Standing
			if err := h.DB.Where("stage_id = ?", st.ID).
				Order("position ASC").
				Find(&standings).Error; err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			item.Standings = h.buildStandingItems(standings)
		}

		items = append(items, item)
	}

	linkKnockoutTree(items)

	return c.JSON(http.StatusOK, echo.Map{
		"status": "success",
		"data":   items,
	})
}

// linkKnockoutTree wires each knock-out tie to the tie its winner plays next,
// then reorders every round so the ties feeding the same later tie sit next to
// each other — which is what lets the UI draw the rounds as a tree instead of
// as unrelated columns.
//
// Both steps are derived from who actually plays whom: the winner of a tie
// turns up in exactly one tie of the following round. Sportmonks stores no
// parent/child link, so nothing here is assumed about seeding or bracket shape.
func linkKnockoutTree(stages []BracketStage) {
	// Collect the knock-out rounds, in playing order.
	var rounds []*BracketStage
	for i := range stages {
		if stages[i].IsKnockout {
			rounds = append(rounds, &stages[i])
		}
	}
	if len(rounds) < 2 {
		return
	}

	// Link every round to the next one.
	for i := 0; i < len(rounds)-1; i++ {
		next := rounds[i+1]
		tieByTeam := make(map[uint]uint) // team id -> tie id in the next round
		for _, t := range next.Ties {
			for _, side := range t.Sides {
				tieByTeam[side.TeamID] = t.ID
			}
		}

		for j := range rounds[i].Ties {
			tie := &rounds[i].Ties[j]
			if tie.WinnerTeamID == nil {
				continue
			}
			if nextID, ok := tieByTeam[*tie.WinnerTeamID]; ok {
				id := nextID
				tie.NextTieID = &id
			}
		}
	}

	// Walking backwards, sort each round by where its ties land in the next one.
	for i := len(rounds) - 2; i >= 0; i-- {
		position := make(map[uint]int, len(rounds[i+1].Ties))
		for idx, t := range rounds[i+1].Ties {
			position[t.ID] = idx
		}

		ties := rounds[i].Ties
		sort.SliceStable(ties, func(a, b int) bool {
			pa, oka := tiePosition(ties[a], position)
			pb, okb := tiePosition(ties[b], position)
			if oka != okb {
				return oka // ties that feed a known later tie come first
			}
			return pa < pb
		})
	}
}

// tiePosition reports where a tie's winner continues in the next round.
func tiePosition(tie BracketTie, position map[uint]int) (int, bool) {
	if tie.NextTieID == nil {
		return 1 << 30, false
	}
	pos, ok := position[*tie.NextTieID]
	if !ok {
		return 1 << 30, false
	}
	return pos, true
}

// stageTypeNames resolves the stage type ids in one query.
func (h *PortalHandler) stageTypeNames(stages []models.Stage) map[uint]string {
	names := make(map[uint]string)
	var ids []uint
	for _, st := range stages {
		if st.TypeID != nil {
			ids = append(ids, *st.TypeID)
		}
	}
	if len(ids) == 0 {
		return names
	}

	var types []models.Type
	h.DB.Where("id IN ?", ids).Find(&types)
	for _, t := range types {
		name := t.Name
		if name == "" {
			name = t.DeveloperName
		}
		names[t.ID] = name
	}
	return names
}

// buildStageTies groups a stage's fixtures into ties. Two fixtures belong to the
// same tie when they are between the same two teams, whichever side was at home,
// which is how a two-legged round is represented in the fixture list.
func (h *PortalHandler) buildStageTies(stageID uint, teamCache map[uint]*models.Team) ([]BracketTie, error) {
	var fixtures []models.Fixture
	if err := h.DB.Where("stage_id = ?", stageID).
		Preload("State").
		Order("starting_at ASC, id ASC").
		Find(&fixtures).Error; err != nil {
		return nil, err
	}
	if len(fixtures) == 0 {
		return []BracketTie{}, nil
	}

	fixtureIDs := make([]uint, 0, len(fixtures))
	for _, f := range fixtures {
		fixtureIDs = append(fixtureIDs, f.ID)
	}

	// Final score per side, keyed by fixture.
	var scores []models.FixtureScore
	h.DB.Where("fixture_id IN ? AND description = ?", fixtureIDs, "CURRENT").Find(&scores)

	type sideScore struct {
		teamID uint
		goals  int
	}
	homeScore := make(map[uint]sideScore)
	awayScore := make(map[uint]sideScore)
	for _, sc := range scores {
		switch strings.ToLower(sc.Participant) {
		case "home":
			homeScore[sc.FixtureID] = sideScore{teamID: sc.ParticipantID, goals: sc.Goals}
		case "away":
			awayScore[sc.FixtureID] = sideScore{teamID: sc.ParticipantID, goals: sc.Goals}
		}
	}

	// Group legs by the unordered pair of teams.
	type tieBuild struct {
		key       string
		order     []uint // team ids, in the order first seen
		aggregate map[uint]int
		legs      []BracketLeg
	}
	byKey := make(map[string]*tieBuild)
	var keyOrder []string

	for _, f := range fixtures {
		home, hasHome := homeScore[f.ID]
		away, hasAway := awayScore[f.ID]
		if !hasHome || !hasAway || home.teamID == 0 || away.teamID == 0 {
			// Without both participants the fixture cannot be paired into a tie.
			continue
		}

		lo, hi := home.teamID, away.teamID
		if lo > hi {
			lo, hi = hi, lo
		}
		key := fmt.Sprintf("%d-%d", lo, hi)

		tb, ok := byKey[key]
		if !ok {
			tb = &tieBuild{key: key, aggregate: map[uint]int{}}
			byKey[key] = tb
			keyOrder = append(keyOrder, key)
			tb.order = []uint{home.teamID, away.teamID}
		}

		played := isPlayedState(f.State)
		if played {
			tb.aggregate[home.teamID] += home.goals
			tb.aggregate[away.teamID] += away.goals
		}

		leg := BracketLeg{
			FixtureID:  f.ID,
			Name:       f.Name,
			StartingAt: f.StartingAt,
			StateID:    f.StateID,
			State:      f.State,
			HomeTeamID: home.teamID,
			AwayTeamID: away.teamID,
			ResultInfo: f.ResultInfo,
		}
		if played {
			hg, ag := home.goals, away.goals
			leg.HomeGoals = &hg
			leg.AwayGoals = &ag
		}
		tb.legs = append(tb.legs, leg)
	}

	ties := make([]BracketTie, 0, len(keyOrder))
	for _, key := range keyOrder {
		tb := byKey[key]

		anyPlayed := false
		for _, leg := range tb.legs {
			if leg.HomeGoals != nil {
				anyPlayed = true
				break
			}
		}

		sides := make([]BracketSide, 0, len(tb.order))
		for _, teamID := range tb.order {
			sides = append(sides, BracketSide{
				Team:      h.teamByID(teamID, teamCache),
				TeamID:    teamID,
				Aggregate: tb.aggregate[teamID],
			})
		}

		tieID := tb.legs[0].FixtureID
		for _, leg := range tb.legs {
			if leg.FixtureID < tieID {
				tieID = leg.FixtureID
			}
		}

		tie := BracketTie{
			ID:         tieID,
			Sides:      sides,
			Legs:       tb.legs,
			Played:     anyPlayed,
			DecidedBy:  "pending",
			ResultInfo: tb.legs[len(tb.legs)-1].ResultInfo,
		}

		if anyPlayed && len(sides) == 2 {
			switch {
			case sides[0].Aggregate > sides[1].Aggregate:
				id := sides[0].TeamID
				tie.WinnerTeamID = &id
				tie.DecidedBy = "aggregate"
			case sides[1].Aggregate > sides[0].Aggregate:
				id := sides[1].TeamID
				tie.WinnerTeamID = &id
				tie.DecidedBy = "aggregate"
			default:
				// Level on aggregate: extra time / penalties settled it, and that
				// outcome only exists as prose in the last leg's result_info.
				tie.DecidedBy = "level"
			}
		}

		ties = append(ties, tie)
	}

	return ties, nil
}

// isPlayedState reports whether a fixture has a final score worth aggregating.
func isPlayedState(state *models.State) bool {
	if state == nil {
		return false
	}
	return finishedFixtureStates[strings.ToUpper(state.ShortName)]
}

// teamByID resolves a team once per request.
func (h *PortalHandler) teamByID(id uint, cache map[uint]*models.Team) *models.Team {
	if id == 0 {
		return nil
	}
	if tm, ok := cache[id]; ok {
		return tm
	}
	var tm models.Team
	if err := h.DB.First(&tm, id).Error; err != nil {
		cache[id] = nil
		return nil
	}
	cache[id] = &tm
	return &tm
}

// ─────────────────────────────────────────────────────────────────────────────
// 3. ROUNDS & FIXTURES (WITH SUB-ENTITIES)
// ─────────────────────────────────────────────────────────────────────────────

// RoundItem represents a round with its fixture count.
type RoundItem struct {
	models.Round
	FixturesCount int64 `json:"fixtures_count"`
}

// GetSeasonRounds returns all rounds for a season.
func (h *PortalHandler) GetSeasonRounds(c echo.Context) error {
	seasonID, err := strconv.Atoi(c.Param("season_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid season id")
	}

	var rounds []models.Round
	if err := h.DB.Where("season_id = ?", seasonID).
		Order("id ASC").
		Find(&rounds).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	var items []RoundItem
	for _, r := range rounds {
		var cnt int64
		h.DB.Model(&models.Fixture{}).Where("round_id = ?", r.ID).Count(&cnt)
		items = append(items, RoundItem{
			Round:         r,
			FixturesCount: cnt,
		})
	}

	return c.JSON(http.StatusOK, echo.Map{
		"status": "success",
		"data":   items,
	})
}

// FixtureListItem represents a fixture with preloaded state, current scores, teams, and venue info.
type FixtureListItem struct {
	models.Fixture
	State            *models.State         `json:"state,omitempty"`
	Scores           []models.FixtureScore `json:"scores"`
	CurrentHomeScore *int                  `json:"current_home_score"`
	CurrentAwayScore *int                  `json:"current_away_score"`
	Events           []models.FixtureEvent `json:"events"`
	HomeTeam         *models.Team          `json:"home_team,omitempty"`
	AwayTeam         *models.Team          `json:"away_team,omitempty"`
	Venue            *models.Venue         `json:"venue,omitempty"`
}

// FixtureStatusItem is one entry of the status filter offered for a season's
// fixture list: the state itself plus how many fixtures the filter would show.
type FixtureStatusItem struct {
	StateID   uint   `json:"state_id"`
	State     string `json:"state"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	Count     int    `json:"count"`
}

// Sportmonks state short names, grouped by how their fixtures should be read.
// Anything not listed here is treated as "other": shown as-is, oldest first.
var (
	// Short names as Sportmonks actually stores them (see the `states` table):
	// a finished-after-penalties match is "FTP", not "FT_PEN", and the halves
	// are "1st"/"2nd" rather than "1H"/"2H". Matching invented codes silently
	// mis-classifies fixtures — an FTP final counted as "not played".
	finishedFixtureStates = map[string]bool{"FT": true, "AET": true, "FTP": true}
	upcomingFixtureStates = map[string]bool{"NS": true, "TBA": true}
	liveFixtureStates     = map[string]bool{"1ST": true, "2ND": true, "HT": true, "BRK": true, "ET": true, "ETB": true, "2ET": true, "PEN": true, "PENB": true}
)

// statusGroupRank orders the status filter chips: live, then upcoming, then
// finished, then everything else.
func statusGroupRank(shortName string) int {
	switch {
	case liveFixtureStates[shortName]:
		return 0
	case upcomingFixtureStates[shortName]:
		return 1
	case finishedFixtureStates[shortName]:
		return 2
	default:
		return 3
	}
}

// GetSeasonFixtures returns fixtures for a season, with optional round_id,
// stage_id and status filters.
//
// The status filter also decides the reading order, because "which match is
// interesting" differs per state: finished fixtures are listed newest first
// (latest results), while not-yet-played ones are listed soonest first and drop
// any kickoff already in the past — those are stale rows the scraper has not
// caught up with, and they would otherwise bury the next real match.
func (h *PortalHandler) GetSeasonFixtures(c echo.Context) error {
	seasonID, err := strconv.Atoi(c.Param("season_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid season id")
	}

	// starting_at is stored as a UTC "Y-m-d H:i:s" string, so a lexicographic
	// comparison against the same format is a valid chronological one.
	nowUTC := time.Now().UTC().Format("2006-01-02 15:04:05")

	// Optional club filter. Upcoming fixtures carry no score rows at all, so the
	// participants are only recoverable from the fixture name ("Home vs Away"),
	// which Sportmonks builds from the very team names we store.
	selectedTeamID, _ := strconv.Atoi(c.QueryParam("team_id"))
	var teamNamePattern string
	if selectedTeamID > 0 {
		var team models.Team
		if err := h.DB.First(&team, selectedTeamID).Error; err != nil || team.Name == "" {
			selectedTeamID = 0
		} else {
			teamNamePattern = team.Name
		}
	}

	// Scope shared by the fixture list and the status counts.
	scope := func() *gorm.DB {
		q := h.DB.Model(&models.Fixture{}).Where("season_id = ?", seasonID)
		if roundID := c.QueryParam("round_id"); roundID != "" {
			q = q.Where("round_id = ?", roundID)
		}
		if stageID := c.QueryParam("stage_id"); stageID != "" {
			q = q.Where("stage_id = ?", stageID)
		}
		if selectedTeamID > 0 {
			q = q.Where("name LIKE ? OR name LIKE ?", teamNamePattern+" vs %", "% vs "+teamNamePattern)
		}
		return q
	}

	availableStatuses, err := h.seasonFixtureStatuses(scope(), nowUTC)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Only honour a status that is a real state; anything else (a typo, a
	// hand-edited URL) falls back to the unfiltered list. A real state with no
	// matching fixtures in this scope still filters — and returns nothing —
	// rather than silently showing every fixture the caller did not ask for.
	selectedStatus := strings.ToUpper(strings.TrimSpace(c.QueryParam("status")))
	if selectedStatus != "" {
		var known int64
		if err := h.DB.Model(&models.State{}).
			Where("UPPER(short_name) = ?", selectedStatus).
			Count(&known).Error; err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if known == 0 {
			selectedStatus = ""
		}
	}

	query := scope().
		Preload("State").
		Preload("Scores").
		Preload("Events").
		Preload("Referees")

	switch {
	case selectedStatus == "":
		query = query.Order("starting_at ASC, id ASC")
	case finishedFixtureStates[selectedStatus]:
		query = query.
			Where("state_id IN (?)", h.stateIDsByShortName(selectedStatus)).
			Order("starting_at DESC, id DESC")
	case upcomingFixtureStates[selectedStatus]:
		query = query.
			Where("state_id IN (?)", h.stateIDsByShortName(selectedStatus)).
			Where("starting_at >= ?", nowUTC).
			Order("starting_at ASC, id ASC")
	default:
		query = query.
			Where("state_id IN (?)", h.stateIDsByShortName(selectedStatus)).
			Order("starting_at ASC, id ASC")
	}

	var fixtures []models.Fixture
	if err := query.Find(&fixtures).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Cache teams for fast lookup
	teamCache := make(map[uint]*models.Team)

	// Non-nil so an empty filter result serialises as [] rather than null —
	// with the status filter, "no matches" is a normal outcome.
	items := make([]FixtureListItem, 0, len(fixtures))
	for _, f := range fixtures {
		var v *models.Venue
		if f.VenueID != nil && *f.VenueID > 0 {
			var venue models.Venue
			if err := h.DB.First(&venue, *f.VenueID).Error; err == nil {
				v = &venue
			}
		}

		// Calculate Current Score
		var curHome, curAway *int
		// First try CURRENT, then 2ND_HALF, then 1ST_HALF
		for _, targetDesc := range []string{"CURRENT", "2ND_HALF", "1ST_HALF"} {
			if curHome != nil && curAway != nil {
				break
			}
			for _, sc := range f.Scores {
				if strings.EqualFold(sc.Description, targetDesc) {
					if sc.Participant == "home" && curHome == nil {
						hVal := sc.Goals
						curHome = &hVal
					} else if sc.Participant == "away" && curAway == nil {
						aVal := sc.Goals
						curAway = &aVal
					}
				}
			}
		}

		// Resolve Home & Away Team
		var homeTeam, awayTeam *models.Team
		for _, sc := range f.Scores {
			if sc.Participant == "home" && sc.ParticipantID > 0 {
				if tm, ok := teamCache[sc.ParticipantID]; ok {
					homeTeam = tm
				} else {
					var t models.Team
					if err := h.DB.First(&t, sc.ParticipantID).Error; err == nil {
						teamCache[sc.ParticipantID] = &t
						homeTeam = &t
					}
				}
			} else if sc.Participant == "away" && sc.ParticipantID > 0 {
				if tm, ok := teamCache[sc.ParticipantID]; ok {
					awayTeam = tm
				} else {
					var t models.Team
					if err := h.DB.First(&t, sc.ParticipantID).Error; err == nil {
						teamCache[sc.ParticipantID] = &t
						awayTeam = &t
					}
				}
			}
		}

		if (homeTeam == nil || awayTeam == nil) && strings.Contains(f.Name, " vs ") {
			parts := strings.Split(f.Name, " vs ")
			if homeTeam == nil {
				var t models.Team
				if err := h.DB.Where("name ILIKE ?", strings.TrimSpace(parts[0])).First(&t).Error; err == nil {
					homeTeam = &t
				}
			}
			if awayTeam == nil && len(parts) > 1 {
				var t models.Team
				if err := h.DB.Where("name ILIKE ?", strings.TrimSpace(parts[1])).First(&t).Error; err == nil {
					awayTeam = &t
				}
			}
		}

		items = append(items, FixtureListItem{
			Fixture:          f,
			State:            f.State,
			Scores:           f.Scores,
			CurrentHomeScore: curHome,
			CurrentAwayScore: curAway,
			Events:           f.Events,
			HomeTeam:         homeTeam,
			AwayTeam:         awayTeam,
			Venue:            v,
		})
	}

	return c.JSON(http.StatusOK, echo.Map{
		"status":             "success",
		"data":               items,
		"available_statuses": availableStatuses,
		"selected_status":    selectedStatus,
		"available_teams":    h.seasonTeamOptions(uint(seasonID)),
		"selected_team_id":   selectedTeamID,
	})
}

// seasonTeamOptions lists the clubs registered for a season, for the fixture
// list's club filter.
func (h *PortalHandler) seasonTeamOptions(seasonID uint) []models.Team {
	var teamIDs []uint
	h.DB.Model(&models.SeasonTeam{}).Where("season_id = ?", seasonID).Pluck("team_id", &teamIDs)

	teams := make([]models.Team, 0, len(teamIDs))
	if len(teamIDs) == 0 {
		return teams
	}

	h.DB.Where("id IN ?", teamIDs).Order("name ASC").Find(&teams)

	return teams
}

// stateIDsByShortName resolves a state short name (FT, NS, …) to the state ids
// carrying it, as a subquery so it stays a single round trip.
func (h *PortalHandler) stateIDsByShortName(shortName string) *gorm.DB {
	return h.DB.Model(&models.State{}).
		Select("id").
		Where("UPPER(short_name) = ?", strings.ToUpper(shortName))
}

// seasonFixtureStatuses lists the states present in the given fixture scope,
// each with the number of fixtures its filter would actually show — upcoming
// states count only kickoffs still ahead, matching what the filter renders.
func (h *PortalHandler) seasonFixtureStatuses(scope *gorm.DB, nowUTC string) ([]FixtureStatusItem, error) {
	var rows []struct {
		StateID    *uint
		StartingAt *string
	}
	if err := scope.Select("state_id, starting_at").Find(&rows).Error; err != nil {
		return nil, err
	}

	var states []models.State
	if err := h.DB.Find(&states).Error; err != nil {
		return nil, err
	}
	stateByID := make(map[uint]models.State, len(states))
	for _, st := range states {
		stateByID[st.ID] = st
	}

	counts := make(map[uint]int)
	for _, r := range rows {
		if r.StateID == nil {
			continue
		}
		st, ok := stateByID[*r.StateID]
		if !ok {
			continue
		}
		if upcomingFixtureStates[strings.ToUpper(st.ShortName)] {
			if r.StartingAt == nil || *r.StartingAt < nowUTC {
				continue
			}
		}
		counts[*r.StateID]++
	}

	items := make([]FixtureStatusItem, 0, len(counts))
	for id, n := range counts {
		st := stateByID[id]
		items = append(items, FixtureStatusItem{
			StateID:   id,
			State:     st.State,
			Name:      st.Name,
			ShortName: st.ShortName,
			Count:     n,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		ri, rj := statusGroupRank(strings.ToUpper(items[i].ShortName)), statusGroupRank(strings.ToUpper(items[j].ShortName))
		if ri != rj {
			return ri < rj
		}
		return items[i].ShortName < items[j].ShortName
	})

	return items, nil
}

// EnrichedEvent represents an event with player names, face photos, assist/sub detail, and home/away flag.
type EnrichedEvent struct {
	models.FixtureEvent
	PlayerName         string `json:"player_name"`
	PlayerImage        string `json:"player_image,omitempty"`
	RelatedPlayerName  string `json:"related_player_name,omitempty"`
	RelatedPlayerImage string `json:"related_player_image,omitempty"`
	IsHome             bool   `json:"is_home"`
	EventTypeName      string `json:"event_type_name"`
}

// PeriodScoreItem represents a comparison of home vs away score for a period.
type PeriodScoreItem struct {
	Description string `json:"description"`
	HomeGoals   int    `json:"home_goals"`
	AwayGoals   int    `json:"away_goals"`
}

// sortStartingXI orders a starting eleven by pitch position: row first, then
// column, falling back to the formation slot when the coordinates are missing.
func sortStartingXI(players []EnrichedLineupPlayer) {
	sort.SliceStable(players, func(i, j int) bool {
		a, b := players[i], players[j]
		if a.Row != b.Row {
			return a.Row < b.Row
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		return formationSlot(a) < formationSlot(b)
	})
}

// sortBench orders substitutes by shirt number, then name.
func sortBench(players []EnrichedLineupPlayer) {
	sort.SliceStable(players, func(i, j int) bool {
		a, b := players[i], players[j]
		an, bn := 1<<30, 1<<30
		if a.JerseyNumber != nil {
			an = *a.JerseyNumber
		}
		if b.JerseyNumber != nil {
			bn = *b.JerseyNumber
		}
		if an != bn {
			return an < bn
		}
		return a.PlayerName < b.PlayerName
	})
}

// formationSlot is the lineup's position index, or a large value when unset.
func formationSlot(p EnrichedLineupPlayer) int {
	if p.FormationPosition != nil {
		return *p.FormationPosition
	}
	return 1 << 30
}

// EnrichedStatisticItem represents a side-by-side metric comparison.
type EnrichedStatisticItem struct {
	TypeID    uint    `json:"type_id"`
	TypeName  string  `json:"type_name"`
	HomeValue float64 `json:"home_value"`
	AwayValue float64 `json:"away_value"`
	HomeText  string  `json:"home_text"`
	AwayText  string  `json:"away_text"`
}

// PlayerInMatchStat represents a specific in-match stat for a player (e.g. Rating, Minutes, Goals, Passes).
type PlayerInMatchStat struct {
	TypeID   uint   `json:"type_id"`
	TypeName string `json:"type_name"`
	Value    string `json:"value"`
}

// EnrichedLineupPlayer represents a player in the lineup with photo, field row/col, captain flag, and in-match statistics.
type EnrichedLineupPlayer struct {
	ID           uint   `json:"id"`
	PlayerID     uint   `json:"player_id"`
	TeamID       uint   `json:"team_id"`
	PlayerName   string `json:"player_name"`
	PlayerImage  string `json:"player_image"`
	JerseyNumber *int   `json:"jersey_number"`
	PositionID   *uint  `json:"position_id"`
	PositionName string `json:"position_name"`
	// DetailedPositionName is the specific role ("Left Back") where Sportmonks
	// supplied one; PositionName stays the coarse GK/DF/MF/FW bucket.
	DetailedPositionID   *uint               `json:"detailed_position_id"`
	DetailedPositionName string              `json:"detailed_position_name"`
	FormationField       string              `json:"formation_field"`
	FormationPosition    *int                `json:"formation_position"`
	Row                  int                 `json:"row"`
	Col                  int                 `json:"col"`
	Rating               *float64            `json:"rating,omitempty"`
	Stats                []PlayerInMatchStat `json:"stats"`
}

// TeamLineupSection groups starting XI and bench for a team with its formation.
type TeamLineupSection struct {
	Team       *models.Team           `json:"team,omitempty"`
	Formation  string                 `json:"formation"`
	StartingXI []EnrichedLineupPlayer `json:"starting_xi"`
	Bench      []EnrichedLineupPlayer `json:"bench"`
}

// formatStatValue formats numerical statistic value cleanly (e.g. 55, 6, 2.5).
func formatStatValue(val float64) string {
	if val == float64(int(val)) {
		return strconv.Itoa(int(val))
	}
	return fmt.Sprintf("%.1f", val)
}

// deriveFormation computes a formation string like "4-3-3" or "4-2-3-1" from lineup fields.
func deriveFormation(startingXI []EnrichedLineupPlayer) string {
	rowCount := make(map[int]int)
	for _, lu := range startingXI {
		if lu.Row > 1 {
			rowCount[lu.Row]++
		}
	}
	if len(rowCount) == 0 {
		return "4-3-3"
	}
	var keys []int
	for k := range rowCount {
		keys = append(keys, k)
	}
	// Sort ascending
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts, strconv.Itoa(rowCount[k]))
	}
	return strings.Join(parts, "-")
}

// GetFixtureDetail returns the complete Match Center for a single fixture.
func (h *PortalHandler) GetFixtureDetail(c echo.Context) error {
	fixtureID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid fixture id")
	}

	var fixture models.Fixture
	if err := h.DB.
		Preload("State").
		Preload("Events", func(db *gorm.DB) *gorm.DB {
			return db.Order("minute ASC, extra_minute ASC")
		}).
		Preload("Lineups").
		Preload("Statistics").
		Preload("Scores").
		Preload("Referees").
		First(&fixture, fixtureID).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "fixture not found")
	}

	var league models.League
	h.DB.First(&league, fixture.LeagueID)

	var season models.Season
	h.DB.First(&season, fixture.SeasonID)

	var venue *models.Venue
	if fixture.VenueID != nil && *fixture.VenueID > 0 {
		var v models.Venue
		if err := h.DB.First(&v, *fixture.VenueID).Error; err == nil {
			venue = &v
		}
	}

	// 1. Identify Home & Away Teams
	var homeTeamID, awayTeamID uint
	for _, sc := range fixture.Scores {
		if sc.Participant == "home" && sc.ParticipantID > 0 {
			homeTeamID = sc.ParticipantID
		} else if sc.Participant == "away" && sc.ParticipantID > 0 {
			awayTeamID = sc.ParticipantID
		}
	}

	// Fallback: detect from lineups if scores didn't have participant_id
	if homeTeamID == 0 || awayTeamID == 0 {
		var teamIDs []uint
		seen := make(map[uint]bool)
		for _, lu := range fixture.Lineups {
			if !seen[lu.TeamID] && lu.TeamID > 0 {
				seen[lu.TeamID] = true
				teamIDs = append(teamIDs, lu.TeamID)
			}
		}
		if len(teamIDs) > 0 && homeTeamID == 0 {
			homeTeamID = teamIDs[0]
		}
		if len(teamIDs) > 1 && awayTeamID == 0 {
			awayTeamID = teamIDs[1]
		}
	}

	var homeTeam, awayTeam models.Team
	if homeTeamID > 0 {
		h.DB.First(&homeTeam, homeTeamID)
	}
	if awayTeamID > 0 {
		h.DB.First(&awayTeam, awayTeamID)
	}

	// Fallback: if name has "Team A vs Team B" and teams not found by ID
	if homeTeam.ID == 0 && strings.Contains(fixture.Name, " vs ") {
		parts := strings.Split(fixture.Name, " vs ")
		h.DB.Where("name ILIKE ?", strings.TrimSpace(parts[0])).First(&homeTeam)
		if len(parts) > 1 {
			h.DB.Where("name ILIKE ?", strings.TrimSpace(parts[1])).First(&awayTeam)
		}
		if homeTeamID == 0 {
			homeTeamID = homeTeam.ID
		}
		if awayTeamID == 0 {
			awayTeamID = awayTeam.ID
		}
	}

	// 2. Pre-cache Player names/photos and Type names
	playerMap := make(map[uint]*models.Player)
	typeMap := make(map[uint]string)
	var allTypes []models.Type
	h.DB.Find(&allTypes)
	for _, t := range allTypes {
		typeMap[t.ID] = t.Name
	}

	getPlayer := func(pid uint) *models.Player {
		if pid == 0 {
			return nil
		}
		if p, ok := playerMap[pid]; ok {
			return p
		}
		var p models.Player
		if err := h.DB.First(&p, pid).Error; err == nil {
			playerMap[pid] = &p
			return &p
		}
		return nil
	}

	// 3. Enrich Match Events
	var enrichedEvents []EnrichedEvent
	for _, ev := range fixture.Events {
		ee := EnrichedEvent{
			FixtureEvent: ev,
		}

		// Resolve Main Player (Scorer or Sub-In)
		if ev.PlayerID != nil && *ev.PlayerID > 0 {
			if p := getPlayer(*ev.PlayerID); p != nil {
				ee.PlayerName = p.DisplayName
				if ee.PlayerName == "" {
					ee.PlayerName = p.Name
				}
				ee.PlayerImage = p.ImagePath
			}
		}
		if ee.PlayerName == "" {
			ee.PlayerName = ev.PlayerName
		}

		// Resolve Related Player (Assist or Sub-Out)
		if ev.RelatedPlayerID != nil && *ev.RelatedPlayerID > 0 {
			if p := getPlayer(*ev.RelatedPlayerID); p != nil {
				ee.RelatedPlayerName = p.DisplayName
				if ee.RelatedPlayerName == "" {
					ee.RelatedPlayerName = p.Name
				}
				ee.RelatedPlayerImage = p.ImagePath
			}
		}

		// Event Type Name
		if tName, ok := typeMap[ev.TypeID]; ok {
			ee.EventTypeName = tName
		} else {
			switch ev.TypeID {
			case 14, 15:
				ee.EventTypeName = "Goal"
			case 16:
				ee.EventTypeName = "Own Goal"
			case 17:
				ee.EventTypeName = "Penalty Missed"
			case 18:
				ee.EventTypeName = "Substitution"
			case 19:
				ee.EventTypeName = "Yellow Card"
			case 20:
				ee.EventTypeName = "Red Card"
			case 21:
				ee.EventTypeName = "Yellow/Red Card"
			default:
				ee.EventTypeName = fmt.Sprintf("Event #%d", ev.TypeID)
			}
		}

		// Determine Home vs Away
		if ev.ParticipantID != nil && *ev.ParticipantID > 0 {
			ee.IsHome = (*ev.ParticipantID == homeTeamID)
		} else {
			ee.IsHome = strings.EqualFold(ev.Section, "home")
		}

		enrichedEvents = append(enrichedEvents, ee)
	}

	// 4. Period Scores Breakdown (ONLY 1ST_HALF, 2ND_HALF, CURRENT)
	periodOrder := []string{"1ST_HALF", "2ND_HALF", "CURRENT"}
	allowedPeriods := map[string]bool{
		"1ST_HALF": true,
		"2ND_HALF": true,
		"CURRENT":  true,
	}
	periodMap := make(map[string]*PeriodScoreItem)
	for _, sc := range fixture.Scores {
		desc := strings.ToUpper(sc.Description)
		if desc == "" {
			desc = "CURRENT"
		}
		if !allowedPeriods[desc] {
			continue
		}
		if _, ok := periodMap[desc]; !ok {
			periodMap[desc] = &PeriodScoreItem{Description: desc}
		}
		if sc.Participant == "home" || (homeTeamID > 0 && sc.ParticipantID == homeTeamID) {
			periodMap[desc].HomeGoals = sc.Goals
		} else {
			periodMap[desc].AwayGoals = sc.Goals
		}
	}
	var periodScores []PeriodScoreItem
	for _, pName := range periodOrder {
		if ps, ok := periodMap[pName]; ok {
			periodScores = append(periodScores, *ps)
		}
	}

	// 5. Statistics Breakdown (Home vs Away Side-by-Side from column 'value')
	statMap := make(map[uint]*EnrichedStatisticItem)
	for _, stat := range fixture.Statistics {
		item, ok := statMap[stat.TypeID]
		if !ok {
			tName := typeMap[stat.TypeID]
			if tName == "" {
				tName = fmt.Sprintf("Metrik #%d", stat.TypeID)
			}
			item = &EnrichedStatisticItem{
				TypeID:   stat.TypeID,
				TypeName: tName,
			}
			statMap[stat.TypeID] = item
		}

		val := 0.0
		if stat.Value != nil {
			val = *stat.Value
		}
		isHome := (stat.Location == "home") || (homeTeamID > 0 && stat.ParticipantID == homeTeamID)

		if isHome {
			item.HomeValue = val
			item.HomeText = formatStatValue(val)
		} else {
			item.AwayValue = val
			item.AwayText = formatStatValue(val)
		}
	}
	var enrichedStatistics []EnrichedStatisticItem
	for _, stat := range statMap {
		enrichedStatistics = append(enrichedStatistics, *stat)
	}

	// 6. Lineups (Home vs Away with 2D Pitch Formations, Photos & In-Match Stats)
	var allLineupDetails []models.FixtureLineupDetail
	h.DB.Where("fixture_id = ?", fixtureID).Find(&allLineupDetails)

	lineupDetailMap := make(map[uint][]models.FixtureLineupDetail)
	for _, dt := range allLineupDetails {
		if dt.PlayerID > 0 {
			lineupDetailMap[dt.PlayerID] = append(lineupDetailMap[dt.PlayerID], dt)
		}
		if dt.LineupID > 0 {
			lineupDetailMap[dt.LineupID] = append(lineupDetailMap[dt.LineupID], dt)
		}
	}

	homeSection := TeamLineupSection{Team: &homeTeam}
	awaySection := TeamLineupSection{Team: &awayTeam}

	for _, lu := range fixture.Lineups {
		elp := EnrichedLineupPlayer{
			ID:                 lu.ID,
			PlayerID:           lu.PlayerID,
			TeamID:             lu.TeamID,
			PlayerName:         lu.PlayerName,
			JerseyNumber:       lu.JerseyNumber,
			PositionID:         lu.PositionID,
			FormationField:     lu.FormationField,
			FormationPosition:  lu.FormationPosition,
			DetailedPositionID: lu.DetailedPositionID,
			Stats:              []PlayerInMatchStat{},
		}

		// Parse field coordinates e.g. "1:1" -> Row: 1, Col: 1
		if strings.Contains(lu.FormationField, ":") {
			parts := strings.Split(lu.FormationField, ":")
			if r, err := strconv.Atoi(parts[0]); err == nil {
				elp.Row = r
			}
			if c, err := strconv.Atoi(parts[1]); err == nil {
				elp.Col = c
			}
		}

		// Lookup Player Photo & Display Name
		if lu.PlayerID > 0 {
			if p := getPlayer(lu.PlayerID); p != nil {
				if elp.PlayerName == "" {
					elp.PlayerName = p.DisplayName
					if elp.PlayerName == "" {
						elp.PlayerName = p.Name
					}
				}
				elp.PlayerImage = p.ImagePath
			}
		}

		// Detailed position ("Left Back") when the scrape captured one.
		if lu.DetailedPositionID != nil && *lu.DetailedPositionID > 0 {
			var dp models.Type
			if err := h.DB.First(&dp, *lu.DetailedPositionID).Error; err == nil {
				elp.DetailedPositionName = dp.Name
			}
		}

		// Position Name
		if lu.PositionID != nil {
			switch *lu.PositionID {
			case 24:
				elp.PositionName = "GK"
			case 25:
				elp.PositionName = "DF"
			case 26:
				elp.PositionName = "MF"
			case 27:
				elp.PositionName = "FW"
			}
		}

		// In-match statistics from FixtureLineupDetail
		details := lineupDetailMap[lu.PlayerID]
		if len(details) == 0 && lu.ID > 0 {
			details = lineupDetailMap[lu.ID]
		}
		seenTypes := make(map[uint]bool)
		for _, dt := range details {
			if seenTypes[dt.TypeID] {
				continue
			}
			seenTypes[dt.TypeID] = true
			tName := typeMap[dt.TypeID]
			if tName == "" {
				tName = fmt.Sprintf("Metrik #%d", dt.TypeID)
			}
			valStr := ""
			if dt.Value != nil {
				valStr = formatStatValue(*dt.Value)
			} else if dt.DataValue != nil {
				valStr = *dt.DataValue
			}
			// Rating comes from type_id 118 (fallback to name match when the type
			// dictionary resolved the name).
			if (dt.TypeID == 118 || strings.Contains(strings.ToLower(tName), "rating")) && dt.Value != nil {
				elp.Rating = dt.Value
			}
			elp.Stats = append(elp.Stats, PlayerInMatchStat{
				TypeID:   dt.TypeID,
				TypeName: tName,
				Value:    valStr,
			})
		}

		isHome := (lu.TeamID == homeTeamID)
		if isHome {
			if lu.FormationPosition != nil {
				homeSection.StartingXI = append(homeSection.StartingXI, elp)
			} else {
				homeSection.Bench = append(homeSection.Bench, elp)
			}
		} else {
			if lu.FormationPosition != nil {
				awaySection.StartingXI = append(awaySection.StartingXI, elp)
			} else {
				awaySection.Bench = append(awaySection.Bench, elp)
			}
		}
	}
	// The XI is rendered as a pitch, so it has to come out in field order:
	// by row, then left-to-right within the row. Without this the players land
	// in whatever order the rows came back from the database, which is why a
	// left back could be drawn in the middle of the defence.
	sortStartingXI(homeSection.StartingXI)
	sortStartingXI(awaySection.StartingXI)
	sortBench(homeSection.Bench)
	sortBench(awaySection.Bench)

	homeSection.Formation = deriveFormation(homeSection.StartingXI)
	awaySection.Formation = deriveFormation(awaySection.StartingXI)

	return c.JSON(http.StatusOK, echo.Map{
		"status": "success",
		"data": echo.Map{
			"fixture":     fixture,
			"state":       fixture.State,
			"league":      league,
			"season":      season,
			"venue":       venue,
			"home_team":   homeTeam,
			"away_team":   awayTeam,
			"events":      enrichedEvents,
			"scores":      periodScores,
			"raw_scores":  fixture.Scores,
			"statistics":  enrichedStatistics,
			"home_lineup": homeSection,
			"away_lineup": awaySection,
			"referees":    fixture.Referees,
		},
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// 4. TEAMS, SQUADS & PLAYERS
// ─────────────────────────────────────────────────────────────────────────────

// TeamWithSquadItem represents a team in a season along with its squad.
type TeamWithSquadItem struct {
	models.Team
	Venue      *models.Venue   `json:"venue,omitempty"`
	SquadCount int             `json:"squad_count"`
	Squad      []models.Squad  `json:"squad,omitempty"`
	Players    []models.Player `json:"players,omitempty"`
}

// GetSeasonTeams returns all teams participating in a season.
func (h *PortalHandler) GetSeasonTeams(c echo.Context) error {
	seasonID, err := strconv.Atoi(c.Param("season_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid season id")
	}

	var seasonTeams []models.SeasonTeam
	h.DB.Where("season_id = ?", seasonID).Find(&seasonTeams)

	teamIDMap := make(map[uint]bool)
	var teamIDs []uint
	for _, st := range seasonTeams {
		if !teamIDMap[st.TeamID] && st.TeamID > 0 {
			teamIDMap[st.TeamID] = true
			teamIDs = append(teamIDs, st.TeamID)
		}
	}

	// Fallback to standings participant_id if season_teams is empty
	if len(teamIDs) == 0 {
		h.DB.Model(&models.Standing{}).Where("season_id = ?", seasonID).Distinct("participant_id").Pluck("participant_id", &teamIDs)
	}

	var teams []models.Team
	if len(teamIDs) > 0 {
		h.DB.Where("id IN ?", teamIDs).Order("name ASC").Find(&teams)
	}

	var items []TeamWithSquadItem
	for _, tm := range teams {
		var v *models.Venue
		if tm.VenueID != nil && *tm.VenueID > 0 {
			var venue models.Venue
			if err := h.DB.First(&venue, *tm.VenueID).Error; err == nil {
				v = &venue
			}
		}

		var squads []models.Squad
		h.DB.Where("team_id = ? AND (season_id = ? OR season_id IS NULL)", tm.ID, seasonID).Find(&squads)

		items = append(items, TeamWithSquadItem{
			Team:       tm,
			Venue:      v,
			SquadCount: len(squads),
		})
	}

	return c.JSON(http.StatusOK, echo.Map{
		"status": "success",
		"data":   items,
	})
}

// GetTeamDetail returns a team's full profile, squad for a season, venue, and rivals.
func (h *PortalHandler) GetTeamDetail(c echo.Context) error {
	teamID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid team id")
	}

	var team models.Team
	if err := h.DB.First(&team, teamID).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "team not found")
	}

	var venue *models.Venue
	if team.VenueID != nil && *team.VenueID > 0 {
		var v models.Venue
		if err := h.DB.First(&v, *team.VenueID).Error; err == nil {
			venue = &v
		}
	}

	// Squad for given season or all squads
	squadQuery := h.DB.Where("team_id = ?", teamID)
	if seasonID := c.QueryParam("season_id"); seasonID != "" {
		squadQuery = squadQuery.Where("season_id = ?", seasonID)
	}
	var squads []models.Squad
	squadQuery.Find(&squads)

	var playerIDs []uint
	for _, sq := range squads {
		if sq.PlayerID > 0 {
			playerIDs = append(playerIDs, sq.PlayerID)
		}
	}

	var players []models.Player
	if len(playerIDs) > 0 {
		h.DB.Where("id IN ?", playerIDs).Find(&players)
	}

	// Resolve position_id -> position name (from the scraped Type dictionary) so
	// the frontend can label squad players instead of hardcoding position ids.
	posIDSet := make(map[uint]bool)
	for _, p := range players {
		if p.PositionID != nil && *p.PositionID > 0 {
			posIDSet[*p.PositionID] = true
		}
	}
	positions := make(map[uint]string)
	if len(posIDSet) > 0 {
		var ids []uint
		for id := range posIDSet {
			ids = append(ids, id)
		}
		var types []models.Type
		h.DB.Where("id IN ?", ids).Find(&types)
		for _, t := range types {
			name := t.Name
			if name == "" {
				name = t.DeveloperName
			}
			positions[t.ID] = name
		}
	}

	// Team Rivals — resolve the "other" team in each pivot into full team rows
	// so the frontend gets names/logos instead of bare ids.
	tid := uint(teamID)
	var rivalPivots []models.TeamRival
	h.DB.Where("team_id = ? OR rival_id = ?", teamID, teamID).Find(&rivalPivots)
	rivalIDSet := make(map[uint]bool)
	for _, rv := range rivalPivots {
		other := rv.RivalID
		if other == tid {
			other = rv.TeamID
		}
		if other > 0 && other != tid {
			rivalIDSet[other] = true
		}
	}
	var rivals []models.Team
	if len(rivalIDSet) > 0 {
		var rivalIDs []uint
		for id := range rivalIDSet {
			rivalIDs = append(rivalIDs, id)
		}
		h.DB.Where("id IN ?", rivalIDs).Find(&rivals)
	}

	// Head coach — prefer the active link, fall back to most recent.
	var coach *models.Coach
	var tc models.TeamCoach
	if err := h.DB.Where("team_id = ?", teamID).Order("active DESC, updated_at DESC").First(&tc).Error; err == nil && tc.CoachID > 0 {
		var ch models.Coach
		if err := h.DB.First(&ch, tc.CoachID).Error; err == nil {
			coach = &ch
		}
	}

	return c.JSON(http.StatusOK, echo.Map{
		"status": "success",
		"data": echo.Map{
			"team":      team,
			"venue":     venue,
			"coach":     coach,
			"squads":    squads,
			"players":   players,
			"positions": positions,
			"rivals":    rivals,
		},
	})
}

// GetPlayerDetail returns detailed player information including bio, position, current teams, transfers, and stats.
func (h *PortalHandler) GetPlayerDetail(c echo.Context) error {
	playerID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid player id")
	}

	var player models.Player
	if err := h.DB.First(&player, playerID).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "player not found")
	}

	var country *models.Country
	if player.CountryID != nil && *player.CountryID > 0 {
		var c models.Country
		if err := h.DB.First(&c, *player.CountryID).Error; err == nil {
			country = &c
		}
	}

	var nationality *models.Country
	if player.NationalityID != nil && *player.NationalityID > 0 {
		var c models.Country
		if err := h.DB.First(&c, *player.NationalityID).Error; err == nil {
			nationality = &c
		}
	}

	// Current squads/teams
	var squads []models.Squad
	h.DB.Where("player_id = ?", playerID).Find(&squads)

	var teamIDs []uint
	for _, sq := range squads {
		if sq.TeamID > 0 {
			teamIDs = append(teamIDs, sq.TeamID)
		}
	}
	var teams []models.Team
	if len(teamIDs) > 0 {
		h.DB.Where("id IN ?", teamIDs).Find(&teams)
	}
	playerTeamByID := make(map[uint]models.Team)
	for _, t := range teams {
		playerTeamByID[t.ID] = t
	}

	// Club history: one row per squad membership (club + season + jersey +
	// captain + current flag). Explains why a player shows more than one club —
	// each is a different season's registration.
	seasonNameByID := make(map[uint]string)
	seasonCurrentByID := make(map[uint]bool)
	{
		seasonSet := make(map[uint]bool)
		for _, sq := range squads {
			if sq.SeasonID != nil && *sq.SeasonID > 0 {
				seasonSet[*sq.SeasonID] = true
			}
		}
		if len(seasonSet) > 0 {
			var ids []uint
			for id := range seasonSet {
				ids = append(ids, id)
			}
			var seasons []models.Season
			h.DB.Where("id IN ?", ids).Find(&seasons)
			for _, s := range seasons {
				seasonNameByID[s.ID] = s.Name
				seasonCurrentByID[s.ID] = s.IsCurrent
			}
		}
	}
	type ClubHistoryItem struct {
		TeamID       uint         `json:"team_id"`
		Team         *models.Team `json:"team,omitempty"`
		SeasonID     uint         `json:"season_id"`
		SeasonName   string       `json:"season_name"`
		JerseyNumber *int         `json:"jersey_number"`
		Captain      bool         `json:"captain"`
		IsCurrent    bool         `json:"is_current"`
	}
	var clubHistory []ClubHistoryItem
	for _, sq := range squads {
		ci := ClubHistoryItem{TeamID: sq.TeamID, JerseyNumber: sq.JerseyNumber, Captain: sq.Captain}
		if sq.SeasonID != nil {
			ci.SeasonID = *sq.SeasonID
			ci.SeasonName = seasonNameByID[*sq.SeasonID]
			ci.IsCurrent = seasonCurrentByID[*sq.SeasonID]
		}
		if t, ok := playerTeamByID[sq.TeamID]; ok {
			tt := t
			ci.Team = &tt
		}
		clubHistory = append(clubHistory, ci)
	}
	// Current seasons first, then most-recent season id.
	sort.Slice(clubHistory, func(i, j int) bool {
		if clubHistory[i].IsCurrent != clubHistory[j].IsCurrent {
			return clubHistory[i].IsCurrent
		}
		return clubHistory[i].SeasonID > clubHistory[j].SeasonID
	})

	// Recent transfers — enriched with from/to team objects + type name so the
	// frontend can render club names and the fee.
	var rawTransfers []models.Transfer
	h.DB.Where("player_id = ?", playerID).Order("date DESC").Find(&rawTransfers)

	type TransferItem struct {
		models.Transfer
		FromTeam *models.Team `json:"from_team,omitempty"`
		ToTeam   *models.Team `json:"to_team,omitempty"`
		TypeName string       `json:"type_name,omitempty"`
	}
	var transfers []TransferItem
	if len(rawTransfers) > 0 {
		teamSet := make(map[uint]bool)
		typeSet := make(map[uint]bool)
		for _, tr := range rawTransfers {
			if tr.FromTeamID != nil {
				teamSet[*tr.FromTeamID] = true
			}
			if tr.ToTeamID != nil {
				teamSet[*tr.ToTeamID] = true
			}
			if tr.TypeID != nil {
				typeSet[*tr.TypeID] = true
			}
		}
		teamByID := make(map[uint]models.Team)
		if len(teamSet) > 0 {
			var ids []uint
			for id := range teamSet {
				ids = append(ids, id)
			}
			var tms []models.Team
			h.DB.Where("id IN ?", ids).Find(&tms)
			for _, t := range tms {
				teamByID[t.ID] = t
			}
		}
		typeNames := make(map[uint]string)
		if len(typeSet) > 0 {
			var ids []uint
			for id := range typeSet {
				ids = append(ids, id)
			}
			var types []models.Type
			h.DB.Where("id IN ?", ids).Find(&types)
			for _, t := range types {
				typeNames[t.ID] = t.Name
			}
		}
		for _, tr := range rawTransfers {
			item := TransferItem{Transfer: tr}
			if tr.FromTeamID != nil {
				if t, ok := teamByID[*tr.FromTeamID]; ok {
					tt := t
					item.FromTeam = &tt
				}
			}
			if tr.ToTeamID != nil {
				if t, ok := teamByID[*tr.ToTeamID]; ok {
					tt := t
					item.ToTeam = &tt
				}
			}
			if tr.TypeID != nil {
				item.TypeName = typeNames[*tr.TypeID]
			}
			transfers = append(transfers, item)
		}
	}

	// Per-season statistics, enriched with season name + team.
	var rawStats []models.PlayerStatistic
	h.DB.Where("player_id = ?", playerID).Find(&rawStats)

	type PlayerStatItem struct {
		models.PlayerStatistic
		SeasonName string       `json:"season_name"`
		Team       *models.Team `json:"team,omitempty"`
	}
	var statistics []PlayerStatItem
	if len(rawStats) > 0 {
		seasonSet := make(map[uint]bool)
		teamSet := make(map[uint]bool)
		for _, st := range rawStats {
			seasonSet[st.SeasonID] = true
			teamSet[st.TeamID] = true
		}
		seasonNames := make(map[uint]string)
		if len(seasonSet) > 0 {
			var ids []uint
			for id := range seasonSet {
				ids = append(ids, id)
			}
			var seasons []models.Season
			h.DB.Where("id IN ?", ids).Find(&seasons)
			for _, s := range seasons {
				seasonNames[s.ID] = s.Name
			}
		}
		teamByID := make(map[uint]models.Team)
		if len(teamSet) > 0 {
			var ids []uint
			for id := range teamSet {
				ids = append(ids, id)
			}
			var tms []models.Team
			h.DB.Where("id IN ?", ids).Find(&tms)
			for _, t := range tms {
				teamByID[t.ID] = t
			}
		}
		for _, st := range rawStats {
			item := PlayerStatItem{PlayerStatistic: st, SeasonName: seasonNames[st.SeasonID]}
			if t, ok := teamByID[st.TeamID]; ok {
				tt := t
				item.Team = &tt
			}
			statistics = append(statistics, item)
		}
	}

	// Topscorer records — enriched with season name, metric (type) name, and team.
	var rawTopscorers []models.Topscorer
	h.DB.Where("player_id = ?", playerID).Find(&rawTopscorers)

	type TopscorerRecord struct {
		models.Topscorer
		SeasonName string       `json:"season_name"`
		TypeName   string       `json:"type_name"`
		Team       *models.Team `json:"team,omitempty"`
	}
	var topscorers []TopscorerRecord
	if len(rawTopscorers) > 0 {
		tsSeasonSet := make(map[uint]bool)
		tsTypeSet := make(map[uint]bool)
		tsTeamSet := make(map[uint]bool)
		for _, ts := range rawTopscorers {
			if ts.SeasonID != nil {
				tsSeasonSet[*ts.SeasonID] = true
			}
			tsTypeSet[ts.TypeID] = true
			if ts.ParticipantID > 0 {
				tsTeamSet[ts.ParticipantID] = true
			}
		}
		tsSeasonName := make(map[uint]string)
		if len(tsSeasonSet) > 0 {
			var ids []uint
			for id := range tsSeasonSet {
				ids = append(ids, id)
			}
			var seasons []models.Season
			h.DB.Where("id IN ?", ids).Find(&seasons)
			for _, s := range seasons {
				tsSeasonName[s.ID] = s.Name
			}
		}
		tsTypeName := make(map[uint]string)
		if len(tsTypeSet) > 0 {
			var ids []uint
			for id := range tsTypeSet {
				ids = append(ids, id)
			}
			var types []models.Type
			h.DB.Where("id IN ?", ids).Find(&types)
			for _, t := range types {
				tsTypeName[t.ID] = t.Name
			}
		}
		tsTeamByID := make(map[uint]models.Team)
		if len(tsTeamSet) > 0 {
			var ids []uint
			for id := range tsTeamSet {
				ids = append(ids, id)
			}
			var tms []models.Team
			h.DB.Where("id IN ?", ids).Find(&tms)
			for _, t := range tms {
				tsTeamByID[t.ID] = t
			}
		}
		for _, ts := range rawTopscorers {
			// Skip records with no season — they can't be grouped and are noise.
			if ts.SeasonID == nil || *ts.SeasonID == 0 {
				continue
			}
			rec := TopscorerRecord{Topscorer: ts, TypeName: tsTypeName[ts.TypeID]}
			rec.SeasonName = tsSeasonName[*ts.SeasonID]
			if t, ok := tsTeamByID[ts.ParticipantID]; ok {
				tt := t
				rec.Team = &tt
			}
			topscorers = append(topscorers, rec)
		}
		// Newest season first, then by rank within a season.
		sort.Slice(topscorers, func(i, j int) bool {
			si := topscorers[i].SeasonID
			sj := topscorers[j].SeasonID
			if si != nil && sj != nil && *si != *sj {
				return *si > *sj
			}
			return topscorers[i].Position < topscorers[j].Position
		})
	}

	// Resolve position + detailed position names from the Type dictionary.
	resolveType := func(id *uint) string {
		if id == nil || *id == 0 {
			return ""
		}
		var t models.Type
		if err := h.DB.First(&t, *id).Error; err == nil {
			if t.Name != "" {
				return t.Name
			}
			return t.DeveloperName
		}
		return ""
	}
	position := resolveType(player.PositionID)
	detailedPosition := resolveType(player.DetailedPositionID)

	return c.JSON(http.StatusOK, echo.Map{
		"status": "success",
		"data": echo.Map{
			"player":            player,
			"country":           country,
			"nationality":       nationality,
			"position":          position,
			"detailed_position": detailedPosition,
			"squads":            squads,
			"teams":             teams,
			"club_history":      clubHistory,
			"transfers":         transfers,
			"topscorers":        topscorers,
			"statistics":        statistics,
		},
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// 5. TOPSCORERS & TRANSFERS
// ─────────────────────────────────────────────────────────────────────────────

// TopscorerItem represents a topscorer row joined with player, team, and metric type info.
type TopscorerItem struct {
	models.Topscorer
	Type   *models.Type   `json:"type,omitempty"`
	Player *models.Player `json:"player,omitempty"`
	Team   *models.Team   `json:"team,omitempty"`
}

// GetSeasonTopscorers returns topscorers for a season, enriched with player, team, and type profiles.
func (h *PortalHandler) GetSeasonTopscorers(c echo.Context) error {
	seasonID, err := strconv.Atoi(c.Param("season_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid season id")
	}

	// 1. Fetch available metric types for this season's topscorers (e.g. Goals, Assists, Yellow Cards, Red Cards)
	var distinctTypeIDs []uint
	h.DB.Model(&models.Topscorer{}).Where("season_id = ?", seasonID).Distinct("type_id").Order("type_id ASC").Pluck("type_id", &distinctTypeIDs)

	var availableTypes []models.Type
	if len(distinctTypeIDs) > 0 {
		h.DB.Where("id IN ?", distinctTypeIDs).Order("id ASC").Find(&availableTypes)
	}

	// Fallback type names if type table not populated
	typeMap := make(map[uint]models.Type)
	for _, t := range availableTypes {
		typeMap[t.ID] = t
	}

	// Determine selected type ID
	selectedTypeID := uint(0)
	if qTypeID := c.QueryParam("type_id"); qTypeID != "" {
		if tid, err := strconv.Atoi(qTypeID); err == nil && tid > 0 {
			selectedTypeID = uint(tid)
		}
	}

	// Default to the goals ranking when it exists — otherwise the lowest
	// type id wins, which is usually a cards ranking (83/84) rather than goals.
	if selectedTypeID == 0 && len(distinctTypeIDs) > 0 {
		selectedTypeID = distinctTypeIDs[0]
		for _, tid := range distinctTypeIDs {
			if tid == goalTopscorerTypeID {
				selectedTypeID = tid
				break
			}
		}
	}

	// 2. Query topscorers for the selected type
	query := h.DB.Where("season_id = ?", seasonID)
	if selectedTypeID > 0 {
		query = query.Where("type_id = ?", selectedTypeID)
	}

	var topscorers []models.Topscorer
	if err := query.Order("position ASC, total DESC").Find(&topscorers).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	var items []TopscorerItem
	for _, ts := range topscorers {
		var pl *models.Player
		var tm *models.Team
		var tp *models.Type

		if t, ok := typeMap[ts.TypeID]; ok {
			tp = &t
		} else {
			var fetchedType models.Type
			if err := h.DB.First(&fetchedType, ts.TypeID).Error; err == nil {
				typeMap[ts.TypeID] = fetchedType
				tp = &fetchedType
			}
		}

		if ts.PlayerID > 0 {
			var player models.Player
			if err := h.DB.First(&player, ts.PlayerID).Error; err == nil {
				pl = &player
			}
		}

		if ts.ParticipantID > 0 {
			var team models.Team
			if err := h.DB.First(&team, ts.ParticipantID).Error; err == nil {
				tm = &team
			}
		}

		items = append(items, TopscorerItem{
			Topscorer: ts,
			Type:      tp,
			Player:    pl,
			Team:      tm,
		})
	}

	return c.JSON(http.StatusOK, echo.Map{
		"status":           "success",
		"selected_type_id": selectedTypeID,
		"available_types":  availableTypes,
		"data":             items,
	})
}

// TransferItem represents a transfer enriched with player and team details.
type TransferItem struct {
	models.Transfer
	Player   *models.Player `json:"player,omitempty"`
	FromTeam *models.Team   `json:"from_team,omitempty"`
	ToTeam   *models.Team   `json:"to_team,omitempty"`
}

// GetSeasonTransfers returns transfers relevant to teams in a season.
func (h *PortalHandler) GetSeasonTransfers(c echo.Context) error {
	seasonID, err := strconv.Atoi(c.Param("season_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid season id")
	}

	var teamIDs []uint
	h.DB.Model(&models.SeasonTeam{}).Where("season_id = ?", seasonID).Distinct("team_id").Pluck("team_id", &teamIDs)
	if len(teamIDs) == 0 {
		h.DB.Model(&models.Standing{}).Where("season_id = ?", seasonID).Distinct("participant_id").Pluck("participant_id", &teamIDs)
	}

	query := h.DB.Model(&models.Transfer{}).Order("date DESC, id DESC").Limit(100)
	if len(teamIDs) > 0 {
		query = query.Where("from_team_id IN ? OR to_team_id IN ?", teamIDs, teamIDs)
	}

	var transfers []models.Transfer
	if err := query.Find(&transfers).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	var items []TransferItem
	for _, tr := range transfers {
		var pl *models.Player
		var ft *models.Team
		var tt *models.Team

		if tr.PlayerID > 0 {
			var p models.Player
			if err := h.DB.First(&p, tr.PlayerID).Error; err == nil {
				pl = &p
			}
		}
		if tr.FromTeamID != nil && *tr.FromTeamID > 0 {
			var f models.Team
			if err := h.DB.First(&f, *tr.FromTeamID).Error; err == nil {
				ft = &f
			}
		}
		if tr.ToTeamID != nil && *tr.ToTeamID > 0 {
			var t models.Team
			if err := h.DB.First(&t, *tr.ToTeamID).Error; err == nil {
				tt = &t
			}
		}

		items = append(items, TransferItem{
			Transfer: tr,
			Player:   pl,
			FromTeam: ft,
			ToTeam:   tt,
		})
	}

	return c.JSON(http.StatusOK, echo.Map{
		"status": "success",
		"data":   items,
	})
}
