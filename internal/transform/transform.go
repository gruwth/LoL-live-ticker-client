// Package transform turns raw Riot payloads into the lean wire format.
// Both functions are pure so they can be tested against a recorded fixture.
package transform

import (
	"math"

	"lolticker-agent/internal/riot"
	"lolticker-agent/internal/wire"
)

// NoEventsSeen is the initial value for the lastSeen cursor of Events.
// It is reset to this on every game start.
const NoEventsSeen = -1

// Snapshot builds the lean per-tick state. Raw item descriptions, spell
// descriptions and runes are dropped here; a snapshot is 2-4 KB where the
// raw payload is 40-80 KB.
func Snapshot(raw *riot.AllGameData, includeActive bool) wire.Snapshot {
	s := wire.Snapshot{
		Game: wire.Game{
			Mode:    raw.GameData.GameMode,
			Time:    round(raw.GameData.GameTime),
			MapName: raw.GameData.MapName,
			Terrain: raw.GameData.MapTerrain,
		},
		Players: make([]wire.Player, 0, len(raw.AllPlayers)),
	}

	for _, p := range raw.AllPlayers {
		wp := wire.Player{
			RiotID:    p.RiotID,
			Name:      p.RiotIDGameName,
			Tag:       p.RiotIDTagLine,
			Champion:  p.ChampionName,
			SkinID:    p.SkinID,
			Team:      p.Team,
			Position:  p.Position,
			Level:     p.Level,
			IsBot:     p.IsBot,
			IsDead:    p.IsDead,
			Respawn:   round(p.RespawnTimer),
			Kills:     p.Scores.Kills,
			Deaths:    p.Scores.Deaths,
			Assists:   p.Scores.Assists,
			CS:        p.Scores.CreepScore,
			WardScore: round(p.Scores.WardScore),
			Items:     make([]wire.Item, 0, len(p.Items)),
			Spells: [2]string{
				p.SummonerSpells.SummonerSpellOne.DisplayName,
				p.SummonerSpells.SummonerSpellTwo.DisplayName,
			},
		}
		wp.Runes = runes(p.Runes)
		for _, it := range p.Items {
			wp.Items = append(wp.Items, wire.Item{ID: it.ItemID, Slot: it.Slot, Count: it.Count})
		}
		s.Players = append(s.Players, wp)
	}

	if includeActive {
		a := raw.ActivePlayer
		s.Active = &wire.Active{
			RiotID: a.RiotID,
			Gold:   round(a.CurrentGold),
			Level:  a.Level,
			HP:     round(a.ChampionStats.CurrentHealth),
			MaxHP:  round(a.ChampionStats.MaxHealth),
			Stats: map[string]float64{
				"ad":          round(a.ChampionStats.AttackDamage),
				"ap":          round(a.ChampionStats.AbilityPower),
				"armor":       round(a.ChampionStats.Armor),
				"as":          round(a.ChampionStats.AttackSpeed),
				"haste":       round(a.ChampionStats.AbilityHaste),
				"mr":          round(a.ChampionStats.MagicResist),
				"ms":          round(a.ChampionStats.MoveSpeed),
				"resource":    round(a.ChampionStats.ResourceValue),
				"resourceMax": round(a.ChampionStats.ResourceMax),
			},
			Abilities: map[string]int{
				"Q": a.Abilities.Q.AbilityLevel,
				"W": a.Abilities.W.AbilityLevel,
				"E": a.Abilities.E.AbilityLevel,
				"R": a.Abilities.R.AbilityLevel,
			},
		}
	}

	return s
}

// Events returns the events that are new since lastSeen, plus the cursor to
// pass in next time. Pass NoEventsSeen on game start.
//
// EventID increments within a game, so normally the cursor is the highest ID
// seen. Some clients report all-zero IDs; when the incoming batch does not
// increase, the cursor falls back to the count of events already emitted.
// A batch whose IDs went backwards means a new game, so everything is re-emitted.
func Events(raw []riot.Event, lastSeen int) (newEvents []wire.Event, newLast int) {
	if len(raw) == 0 {
		return nil, lastSeen
	}

	if idsUsable(raw) {
		maxID := raw[len(raw)-1].EventID
		if maxID < lastSeen {
			lastSeen = NoEventsSeen // game restarted, IDs went backwards
		}
		newLast = lastSeen
		for _, e := range raw {
			if e.EventID <= lastSeen {
				continue
			}
			newEvents = append(newEvents, convert(e))
			if e.EventID > newLast {
				newLast = e.EventID
			}
		}
		return newEvents, newLast
	}

	// Fallback: IDs carry no information, so treat lastSeen as a count.
	seen := lastSeen
	if seen < 0 || seen > len(raw) {
		seen = 0
	}
	for _, e := range raw[seen:] {
		newEvents = append(newEvents, convert(e))
	}
	return newEvents, len(raw)
}

// idsUsable reports whether EventIDs actually identify events: they must be
// non-decreasing across the batch and not all zero. A single event is taken at
// face value, otherwise the very first tick of a game (one event, ID 0) would
// start on the count fallback and then lose an event when it switched over.
func idsUsable(raw []riot.Event) bool {
	for i := 1; i < len(raw); i++ {
		if raw[i].EventID < raw[i-1].EventID {
			return false
		}
	}
	return raw[len(raw)-1].EventID > 0 || len(raw) == 1
}

// runes maps the rune block allgamedata already carries. It returns nil when
// the client has not filled it in yet, so the field is simply absent rather
// than present and empty.
func runes(r riot.Runes) *wire.Runes {
	if r.Keystone.ID == 0 && r.Keystone.DisplayName == "" {
		return nil
	}
	return &wire.Runes{
		Keystone:    r.Keystone.DisplayName,
		KeystoneID:  r.Keystone.ID,
		PrimaryTree: r.PrimaryRuneTree.DisplayName,
		SecondTree:  r.SecondaryRuneTree.DisplayName,
	}
}

// round trims the float32-widened-to-float64 noise the client sends
// (0.6439999938011169 for an attack speed of 0.644). Two decimals is more
// precision than anything on screen needs and keeps the payload small.
func round(f float64) float64 {
	return math.Round(f*100) / 100
}

func convert(e riot.Event) wire.Event {
	return wire.Event{
		ID:        e.EventID,
		Name:      e.EventName,
		Time:      e.EventTime,
		Killer:    e.KillerName,
		Victim:    e.VictimName,
		Assists:   e.Assisters,
		Dragon:    e.DragonType,
		Stolen:    e.Stolen == "True",
		Turret:    e.TurretKilled,
		Inhib:     e.InhibKilled,
		Streak:    e.KillStreak,
		Acer:      e.Acer,
		AcingTeam: e.AcingTeam,
		Result:    e.Result,
	}
}
