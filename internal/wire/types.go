// Package wire is the contract between agent, relay and frontend.
// Everything the agent sends is defined here - if it is not in this file,
// it does not leave the machine.
package wire

import "encoding/json"

// Version is the protocol version carried in every envelope.
const Version = 1

// Envelope types.
const (
	TypeSnapshot = "snapshot"
	TypeEvents   = "events"
	TypeGameEnd  = "gameEnd"
	TypeIdle     = "idle"
)

// Envelope is one JSON message per WebSocket text frame, agent -> relay.
type Envelope struct {
	V    int             `json:"v"`    // protocol version, currently 1
	Type string          `json:"type"` // "snapshot" | "events" | "gameEnd" | "idle"
	Seq  int64           `json:"seq"`  // monotonic per connection, starts at 1
	TS   int64           `json:"ts"`   // agent unix millis, for latency display
	Data json.RawMessage `json:"data,omitempty"`
}

type Snapshot struct {
	Game    Game     `json:"game"`
	Players []Player `json:"players"`
	Active  *Active  `json:"active,omitempty"` // nil if user opted out
}

type Game struct {
	Mode    string  `json:"mode"`
	Time    float64 `json:"time"`
	MapName string  `json:"map"`
	Terrain string  `json:"terrain"`
	Version string  `json:"version,omitempty"` // patch, for Data Dragon asset URLs
}

type Player struct {
	RiotID    string    `json:"riotId"`
	Name      string    `json:"name"` // riotIdGameName
	Tag       string    `json:"tag"`
	Champion  string    `json:"champion"` // championName
	SkinID    int       `json:"skinId"`
	Team      string    `json:"team"` // ORDER | CHAOS
	Position  string    `json:"pos,omitempty"`
	Level     int       `json:"lvl"`
	IsBot     bool      `json:"bot,omitempty"`
	IsDead    bool      `json:"dead,omitempty"`
	Respawn   float64   `json:"respawn,omitempty"`
	Kills     int       `json:"k"`
	Deaths    int       `json:"d"`
	Assists   int       `json:"a"`
	CS        int       `json:"cs"`
	WardScore float64   `json:"ws"`
	Items     []Item    `json:"items"`
	Spells    [2]string `json:"spells"` // display names, e.g. ["Flash","Ignite"]
	Runes     *Runes    `json:"runes,omitempty"`
}

type Runes struct {
	Keystone    string `json:"keystone"`   // display name, "Conqueror"
	KeystoneID  int    `json:"keystoneId"` // for icon lookup
	PrimaryTree string `json:"primary"`    // "Precision"
	SecondTree  string `json:"secondary"`  // "Domination"
}

// Item carries no price: the frontend resolves id against Data Dragon
// item.json for gold.total, which is the right number for the lead estimate.
type Item struct {
	ID    int `json:"id"`
	Slot  int `json:"slot"`
	Count int `json:"count"`
}

type Active struct {
	RiotID    string             `json:"riotId"`
	Gold      float64            `json:"gold"`
	Level     int                `json:"lvl"`
	HP        float64            `json:"hp"`
	MaxHP     float64            `json:"maxHp"`
	Stats     map[string]float64 `json:"stats,omitempty"`
	Abilities map[string]int     `json:"abilities,omitempty"` // {"Q":3,"W":1,...}
}

type Event struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Time      float64  `json:"t"`
	Killer    string   `json:"killer,omitempty"`
	Victim    string   `json:"victim,omitempty"`
	Assists   []string `json:"assists,omitempty"`
	Dragon    string   `json:"dragon,omitempty"`
	Stolen    bool     `json:"stolen,omitempty"` // parsed from "True"/"False"
	Turret    string   `json:"turret,omitempty"`
	Inhib     string   `json:"inhib,omitempty"`
	Streak    int      `json:"streak,omitempty"`
	Acer      string   `json:"acer,omitempty"`
	AcingTeam string   `json:"acingTeam,omitempty"`
	Result    string   `json:"result,omitempty"`
	Synthetic bool     `json:"syn,omitempty"`   // agent-inferred, not reported by Riot
	Quest     string   `json:"quest,omitempty"` // "top" | "mid" | "bot"
}

// GameEnd is the payload of a gameEnd envelope: {"result":"Win"} or {}.
type GameEnd struct {
	Result string `json:"result,omitempty"`
}

// ServerMessage is everything the agent understands from relay -> agent.
type ServerMessage struct {
	Type string `json:"type"` // "ok" | "error"
	Code string `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`
	// ShareURL is the public page for this agent. The relay only derives a
	// slug once a Riot ID is bound, so it is empty before the first game.
	ShareURL string `json:"shareUrl,omitempty"`
}
