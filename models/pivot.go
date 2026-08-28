package models

import (
	"encoding/json"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// Pivot Tables (Many-to-Many join tables)
// ──────────────────────────────────────────────────────────────────────────────

// SeasonTeam is the pivot table for Season <-> Team (Many-to-Many).
type SeasonTeam struct {
	SeasonID uint `json:"season_id" gorm:"primaryKey"`
	TeamID   uint `json:"team_id" gorm:"primaryKey"`
}

// PlayerSeason is the pivot table for Player <-> Season (Many-to-Many).
type PlayerSeason struct {
	PlayerID uint `json:"player_id" gorm:"primaryKey"`
	SeasonID uint `json:"season_id" gorm:"primaryKey"`
	TeamID   uint `json:"team_id" gorm:"index"`
}

// TeamRival is the pivot table for Team <-> Rival Team (Many-to-Many self-referencing).
type TeamRival struct {
	ID      uint `json:"id" gorm:"primaryKey"`
	SportID uint `json:"sport_id"`
	TeamID  uint `json:"team_id" gorm:"index"`
	RivalID uint `json:"rival_id" gorm:"index"`
}

// TeamCoach links a team to its coach(es), captured from the team `coaches`
// include during team scraping. Composite PK keeps upserts idempotent. Written
// only by the team scraper, so a plain Team upsert elsewhere never clobbers it.
type TeamCoach struct {
	TeamID    uint      `json:"team_id" gorm:"primaryKey;autoIncrement:false;index"`
	CoachID   uint      `json:"coach_id" gorm:"primaryKey;autoIncrement:false"`
	Active    bool      `json:"active"`
	Start     *string   `json:"start"`
	End       *string   `json:"end"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Fixture Sub-Entities (one-to-many from Fixture, but also pivot/M2M)
// ──────────────────────────────────────────────────────────────────────────────

// FixtureEvent represents match events: goals, cards, substitutions, VAR, penalties, etc.
// Sportmonks include: ?include=events
type FixtureEvent struct {
	ID              uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	FixtureID       uint      `json:"fixture_id" gorm:"index"`
	PeriodID        *uint     `json:"period_id"`
	ParticipantID   *uint     `json:"participant_id" gorm:"index"`
	TypeID          uint      `json:"type_id" gorm:"index"`
	Section         string    `json:"section"`
	PlayerID        *uint     `json:"player_id" gorm:"index"`
	RelatedPlayerID *uint     `json:"related_player_id"`
	PlayerName      string    `json:"player_name"`
	Result          *string   `json:"result"`
	Info            *string   `json:"info"`
	Addition        *string   `json:"addition"`
	Minute          *int      `json:"minute"`
	ExtraMinute     *int      `json:"extra_minute"`
	Injured         *bool     `json:"injured"`
	OnBench         *bool     `json:"on_bench"`
	CoachID         *uint     `json:"coach_id"`
	SubTypeID       *uint     `json:"sub_type_id"`
	SortOrder       int       `json:"sort_order"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// FixtureLineup represents the lineup for a fixture (starting XI + bench).
// Sportmonks include: ?include=lineups.details
type FixtureLineup struct {
	ID                 uint                  `json:"id" gorm:"primaryKey;autoIncrement:false"`
	SportID            uint                  `json:"sport_id"`
	FixtureID          uint                  `json:"fixture_id" gorm:"index"`
	PlayerID           uint                  `json:"player_id" gorm:"index"`
	TeamID             uint                  `json:"team_id" gorm:"index"`
	PositionID         *uint                 `json:"position_id"`
	DetailedPositionID *uint                 `json:"detailed_position_id"`
	TypeID             *uint                 `json:"type_id"`
	FormationField     string                `json:"formation_field"`
	FormationPosition  *int                  `json:"formation_position"`
	PlayerName         string                `json:"player_name"`
	JerseyNumber       *int                  `json:"jersey_number"`
	CreatedAt          time.Time             `json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
	Details            []FixtureLineupDetail `json:"details,omitempty" gorm:"foreignKey:LineupID;constraint:false;"`
}

// FixtureLineupDetail represents in-match player statistics/performance metrics
// (e.g. rating, minutes played, goals, assists, shots, passes, tackles, fouls, cards).
// Sportmonks include: ?include=lineups.details
type FixtureLineupDetail struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	FixtureID uint      `json:"fixture_id" gorm:"index"`
	LineupID  uint      `json:"lineup_id" gorm:"index"`
	PlayerID  uint      `json:"player_id" gorm:"index"`
	TypeID    uint      `json:"type_id" gorm:"index"`
	Value     *float64  `json:"value" gorm:"column:value"`
	DataValue *string   `json:"data_value" gorm:"column:data_value;type:jsonb"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (d *FixtureLineupDetail) UnmarshalJSON(data []byte) error {
	type Alias FixtureLineupDetail
	aux := &struct {
		Data json.RawMessage `json:"data"`
		*Alias
	}{
		Alias: (*Alias)(d),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.Data) > 0 && string(aux.Data) != "null" {
		str := string(aux.Data)
		d.DataValue = &str

		var valObj struct {
			Value *float64 `json:"value"`
		}
		if err := json.Unmarshal(aux.Data, &valObj); err == nil && valObj.Value != nil {
			d.Value = valObj.Value
		} else {
			var num float64
			if err := json.Unmarshal(aux.Data, &num); err == nil {
				d.Value = &num
			}
		}
	}
	return nil
}

// FixtureStatistic represents team-level statistics for a fixture (possession, shots, passes, etc.).
// Sportmonks include: ?include=statistics
type FixtureStatistic struct {
	ID            uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	FixtureID     uint      `json:"fixture_id" gorm:"index"`
	TypeID        uint      `json:"type_id" gorm:"index"`
	ParticipantID uint      `json:"participant_id" gorm:"index"`
	DataValue     *string   `json:"data_value" gorm:"column:data_value;type:jsonb"`
	Value         *float64  `json:"value" gorm:"column:value"`
	Location      string    `json:"location"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (s *FixtureStatistic) UnmarshalJSON(data []byte) error {
	type Alias FixtureStatistic
	aux := &struct {
		Data json.RawMessage `json:"data"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.Data) > 0 && string(aux.Data) != "null" {
		str := string(aux.Data)
		s.DataValue = &str

		var valObj struct {
			Value *float64 `json:"value"`
		}
		if err := json.Unmarshal(aux.Data, &valObj); err == nil && valObj.Value != nil {
			s.Value = valObj.Value
		}
	}
	return nil
}

// FixtureScore represents the score breakdown by period (HT, FT, ET, PEN).
// Sportmonks include: ?include=scores
type FixtureScore struct {
	ID            uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	FixtureID     uint      `json:"fixture_id" gorm:"index"`
	TypeID        uint      `json:"type_id" gorm:"index"`
	ParticipantID uint      `json:"participant_id" gorm:"index"`
	Goals         int       `json:"goals" gorm:"column:goals"`
	Participant   string    `json:"participant" gorm:"column:participant"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (s *FixtureScore) UnmarshalJSON(data []byte) error {
	type Alias FixtureScore
	aux := &struct {
		Score struct {
			Goals       int    `json:"goals"`
			Participant string `json:"participant"`
		} `json:"score"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err == nil {
		s.Goals = aux.Score.Goals
		s.Participant = aux.Score.Participant
		return nil
	}

	// Fallback in case API returns unexpected structure
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if id, ok := raw["id"].(float64); ok {
		s.ID = uint(id)
	}
	if fid, ok := raw["fixture_id"].(float64); ok {
		s.FixtureID = uint(fid)
	}
	if tid, ok := raw["type_id"].(float64); ok {
		s.TypeID = uint(tid)
	}
	if pid, ok := raw["participant_id"].(float64); ok {
		s.ParticipantID = uint(pid)
	}
	if desc, ok := raw["description"].(string); ok {
		s.Description = desc
	}
	if sc, ok := raw["score"].(map[string]interface{}); ok {
		if g, ok := sc["goals"].(float64); ok {
			s.Goals = int(g)
		}
		if p, ok := sc["participant"].(string); ok {
			s.Participant = p
		}
	} else if g, ok := raw["score"].(float64); ok {
		s.Goals = int(g)
	}
	return nil
}

// FixtureReferee is the pivot table for Fixture <-> Referee (Many-to-Many).
// Sportmonks include: ?include=referees
type FixtureReferee struct {
	FixtureID uint  `json:"fixture_id" gorm:"primaryKey"`
	RefereeID uint  `json:"referee_id" gorm:"primaryKey"`
	TypeID    *uint `json:"type_id"`
}

// Commentary represents live text commentary for a fixture.
// Sportmonks endpoint: /football/commentaries/fixtures/:fixtureId
type Commentary struct {
	ID          uint   `json:"id" gorm:"primaryKey;autoIncrement:false"`
	FixtureID   uint   `json:"fixture_id" gorm:"index"`
	Comment     string `json:"comment"`
	Minute      *int   `json:"minute"`
	ExtraMinute *int   `json:"extra_minute"`
	IsGoal      bool   `json:"is_goal"`
	IsImportant bool   `json:"is_important"`
	Order       int    `json:"order"`
}

// State represents the match state (e.g. NS, 1H, HT, 2H, FT, ET, PEN, etc.).
// Sportmonks endpoint: /football/states
type State struct {
	ID            uint   `json:"id" gorm:"primaryKey;autoIncrement:false"`
	State         string `json:"state"`
	Name          string `json:"name"`
	ShortName     string `json:"short_name"`
	DeveloperName string `json:"developer_name"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Season-level Statistics
// ──────────────────────────────────────────────────────────────────────────────

// PlayerStatistic stores aggregated player performance stats per season/team.
type PlayerStatistic struct {
	// Composite PK (player_id, season_id, team_id) keeps the per-season aggregate
	// idempotent across re-scrapes instead of inserting a new auto-id row each run.
	PlayerID    uint      `json:"player_id" gorm:"primaryKey;autoIncrement:false;index"`
	SeasonID    uint      `json:"season_id" gorm:"primaryKey;autoIncrement:false;index"`
	TeamID      uint      `json:"team_id" gorm:"primaryKey;autoIncrement:false;index"`
	PositionID  *uint     `json:"position_id"`
	Goals       int       `json:"goals"`
	Assists     int       `json:"assists"`
	Appearances int       `json:"appearances"`
	Lineups     int       `json:"lineups"`
	Minutes     int       `json:"minutes"`
	YellowCards int       `json:"yellow_cards"`
	RedCards    int       `json:"red_cards"`
	Rating      *float64  `json:"rating"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
