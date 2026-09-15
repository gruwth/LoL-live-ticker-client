package lcu

// Session is /lol-champ-select/v1/session, narrowed to the fields this agent
// reads. Everything omitted here — puuids, summoner IDs, skins, spells, the
// timer — is deliberately not decoded.
type Session struct {
	// Actions is the source of truth for what has actually happened. Each
	// inner slice is one round of the draft.
	Actions           [][]Action `json:"actions"`
	MyTeam            []Player   `json:"myTeam"`
	TheirTeam         []Player   `json:"theirTeam"`
	LocalPlayerCellID int        `json:"localPlayerCellId"`
	IsCustomGame      bool       `json:"isCustomGame"`
	IsSpectating      bool       `json:"isSpectating"`
}

type Action struct {
	ActorCellID int    `json:"actorCellId"`
	ChampionID  int    `json:"championId"`
	Completed   bool   `json:"completed"`
	Type        string `json:"type"` // "pick" | "ban" | "ten_bans_reveal"
}

// Player is one seat in champ select.
//
// ChampionID and ChampionPickIntent are decoded only so it is obvious they are
// never read: a hover lives in one or both of them, and the pick is taken from
// a completed action instead. See transform.ChampSelect.
type Player struct {
	CellID             int    `json:"cellId"`
	ChampionID         int    `json:"championId"`
	ChampionPickIntent int    `json:"championPickIntent"`
	AssignedPosition   string `json:"assignedPosition"`
	NameVisibilityType string `json:"nameVisibilityType"`
}

// CompletedPicks maps cell ID to the champion locked in that seat. A champion
// that is only hovered never appears, because a hover has no completed action.
func (s *Session) CompletedPicks() map[int]int {
	out := map[int]int{}
	for _, round := range s.Actions {
		for _, a := range round {
			if a.Type == "pick" && a.Completed && a.ChampionID != 0 {
				out[a.ActorCellID] = a.ChampionID
			}
		}
	}
	return out
}

// Bans returns every champion banned so far. Bans carry no identity and are
// mutual knowledge the moment they land, so they travel freely.
func (s *Session) Bans() []int {
	var out []int
	seen := map[int]bool{}
	for _, round := range s.Actions {
		for _, a := range round {
			if a.Type == "ban" && a.Completed && a.ChampionID != 0 && !seen[a.ChampionID] {
				seen[a.ChampionID] = true
				out = append(out, a.ChampionID)
			}
		}
	}
	return out
}
