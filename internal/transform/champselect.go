package transform

import (
	"fmt"
	"strings"

	"lolticker-agent/internal/lcu"
	"lolticker-agent/internal/wire"
)

// Phase maps an LCU gameflow phase onto the wire value. Unknown phases are
// passed through lowercased rather than dropped: Riot adds them, and a value
// the frontend does not recognise is better than silence.
func Phase(raw string) wire.Phase {
	switch raw {
	case "None", "":
		return wire.Phase{Phase: "none"}
	case "Lobby":
		return wire.Phase{Phase: "lobby"}
	case "Matchmaking":
		return wire.Phase{Phase: "matchmaking"}
	case "ReadyCheck":
		return wire.Phase{Phase: "readycheck"}
	case "ChampSelect":
		return wire.Phase{Phase: "champselect"}
	case "GameStart", "InProgress", "Reconnect":
		return wire.Phase{Phase: "ingame"}
	case "WaitingForStats", "PreEndOfGame", "EndOfGame":
		return wire.Phase{Phase: "endofgame"}
	default:
		return wire.Phase{Phase: strings.ToLower(raw)}
	}
}

// ChampSelect builds the champ select message.
//
// Two rules are enforced structurally rather than by filtering, because data
// that never leaves the machine cannot leak:
//
// Picks come only from completed actions. A hovered champion has no completed
// action, so there is no path by which a hover reaches the wire — Player's own
// championId and championPickIntent fields are never read. Locked picks are
// already mutual knowledge to both teams as they lock, so publishing them
// creates no asymmetry; hovers are visible only to a player's own team.
//
// Names are never copied from the session at all. Only the seat the local
// player occupies is labelled with anything identifying, and even that comes
// from the caller rather than the payload. Everyone else gets a positional
// label, in every queue — behaviour that varies by queue type is behaviour
// that will be wrong somewhere.
func ChampSelect(s *lcu.Session, ownerRiotID string) wire.ChampSelect {
	picks := s.CompletedPicks()

	out := wire.ChampSelect{
		Bans:      s.Bans(),
		MyTeam:    seats(s.MyTeam, picks, "Ally", s.LocalPlayerCellID, ownerRiotID),
		TheirTeam: seats(s.TheirTeam, picks, "Enemy", -1, ""),
	}
	if out.Bans == nil {
		out.Bans = []int{}
	}
	return out
}

func seats(players []lcu.Player, picks map[int]int, prefix string, ownerCell int, ownerRiotID string) []wire.CSPlayer {
	out := make([]wire.CSPlayer, 0, len(players))
	for i, p := range players {
		champ, locked := picks[p.CellID]
		seat := wire.CSPlayer{
			Slot:       i,
			Label:      fmt.Sprintf("%s %d", prefix, i+1),
			ChampionID: champ,
			Locked:     locked,
		}
		if ownerCell >= 0 && p.CellID == ownerCell {
			seat.IsOwner = true
			if ownerRiotID != "" {
				seat.Label = ownerRiotID
			}
		}
		out = append(out, seat)
	}
	return out
}
