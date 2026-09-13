// Package quest infers the 2026 role-quest completions.
//
// The quests progress passively and fire no event of their own, but all three
// meaningful completions are observable in data the agent already polls. This
// lives in the agent rather than the frontend so the completion lands in the
// relay's stored event list: a viewer who joins at minute 30 still sees that
// the top laner finished at 14:20, which frontend-side detection could never
// show them.
package quest

import (
	"strings"

	"lolticker-agent/internal/riot"
	"lolticker-agent/internal/wire"
)

// EventName is the stable name every synthetic quest event carries.
const EventName = "QuestComplete"

// FirstID starts the synthetic ID space. It is deliberately far above any
// Riot EventID so the two can never collide, and synthetic events never go
// through the high-water-mark dedup in transform.Events.
const FirstID = 1_000_000

// Quest names as they go on the wire.
const (
	Top = "top"
	Mid = "mid"
	Bot = "bot"
)

// Tracker remembers which quests it has already reported. Jungle and support
// got tuning rather than new rewards in 26.01, so there is no observable state
// change and nothing is emitted for them.
type Tracker struct {
	emitted map[string]bool // riotID + "/" + quest
	nextID  int
}

func NewTracker() *Tracker {
	return &Tracker{emitted: map[string]bool{}, nextID: FirstID}
}

// Reset clears everything for a new game.
func (t *Tracker) Reset() {
	t.emitted = map[string]bool{}
	t.nextID = FirstID
}

// Check returns an event for every quest that is newly observed as complete.
// Each quest fires once per player per game: an item sold and rebought must
// not produce a second event, so completion is latched, never re-derived.
func (t *Tracker) Check(players []riot.Player, gameTime float64) []wire.Event {
	var out []wire.Event
	for _, p := range players {
		if p.RiotID == "" {
			continue
		}
		for _, q := range []struct {
			name string
			done bool
		}{
			{Top, topDone(p)},
			{Mid, midDone(p)},
			{Bot, botDone(p)},
		} {
			if !q.done {
				continue
			}
			key := p.RiotID + "/" + q.name
			if t.emitted[key] {
				continue
			}
			t.emitted[key] = true
			out = append(out, wire.Event{
				ID:        t.nextID,
				Name:      EventName,
				Time:      gameTime,
				Killer:    p.RiotID,
				Synthetic: true,
				Quest:     q.name,
			})
			t.nextID++
		}
	}
	return out
}

// topDone: the quest upgrades Teleport and raises the level cap. Either is
// enough, because a top laner need not have taken Teleport at all.
func topDone(p riot.Player) bool {
	if p.Level > MaxLevelWithoutQuest {
		return true
	}
	for _, k := range []string{
		SpellKey(p.SummonerSpells.SummonerSpellOne),
		SpellKey(p.SummonerSpells.SummonerSpellTwo),
	} {
		if strings.Contains(k, "Teleport") && k != BaseTeleportKey {
			return true
		}
	}
	return false
}

// midDone: the quest grants tier-3 boots.
func midDone(p riot.Player) bool {
	for _, it := range p.Items {
		if Tier3Boots[it.ItemID] {
			return true
		}
	}
	return false
}

// botDone: the quest grants a seventh item slot on top of six items plus a
// trinket.
func botDone(p riot.Player) bool { return len(p.Items) > BotQuestItemCount }

// SpellKey pulls the Data Dragon key out of the localisation string the client
// sends, turning "GeneratedTip_SummonerSpell_SummonerTeleport_DisplayName"
// into "SummonerTeleport". rawDisplayName is used rather than displayName
// because the display name is localised and does not distinguish an upgraded
// variant from its base spell.
func SpellKey(s riot.SummonerSpell) string {
	k := strings.TrimPrefix(s.RawDisplayName, "GeneratedTip_SummonerSpell_")
	return strings.TrimSuffix(k, "_DisplayName")
}
