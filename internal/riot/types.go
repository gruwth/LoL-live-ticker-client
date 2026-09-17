package riot

// Structs mirror the raw responses of https://127.0.0.1:2999/liveclientdata.
// Field names are those the current client actually sends: summonerName is
// empty or absent on Riot ID accounts, so only riotId* is read.

type AllGameData struct {
	ActivePlayer ActivePlayer `json:"activePlayer"`
	AllPlayers   []Player     `json:"allPlayers"`
	Events       Events       `json:"events"`
	GameData     GameData     `json:"gameData"`
}

type GameData struct {
	GameMode   string  `json:"gameMode"` // CLASSIC, ARAM, PRACTICETOOL, ...
	GameTime   float64 `json:"gameTime"` // seconds, stops during pause
	MapName    string  `json:"mapName"`  // Map11, Map12
	MapNumber  int     `json:"mapNumber"`
	MapTerrain string  `json:"mapTerrain"` // Default, Infernal, Mountain, ...
}

type Player struct {
	ChampionName    string         `json:"championName"`
	IsBot           bool           `json:"isBot"`
	IsDead          bool           `json:"isDead"`
	Items           []Item         `json:"items"`
	Level           int            `json:"level"`
	Position        string         `json:"position"` // TOP/JUNGLE/MIDDLE/BOTTOM/UTILITY, often ""
	RawChampionName string         `json:"rawChampionName"`
	RespawnTimer    float64        `json:"respawnTimer"`
	RiotID          string         `json:"riotId"` // "Name#TAG"
	RiotIDGameName  string         `json:"riotIdGameName"`
	RiotIDTagLine   string         `json:"riotIdTagLine"`
	Runes           Runes          `json:"runes"`
	Scores          Scores         `json:"scores"`
	SkinID          int            `json:"skinID"`
	SummonerSpells  SummonerSpells `json:"summonerSpells"`
	Team            string         `json:"team"` // ORDER (blue) | CHAOS (red)
}

// Runes is carried inside allgamedata for every player, so the separate
// /playermainrunes endpoint is not needed. Runes cannot change during a game.
type Runes struct {
	Keystone          Rune `json:"keystone"`
	PrimaryRuneTree   Rune `json:"primaryRuneTree"`
	SecondaryRuneTree Rune `json:"secondaryRuneTree"`
}

type Rune struct {
	DisplayName string `json:"displayName"`
	ID          int    `json:"id"`
}

type Scores struct {
	Assists    int     `json:"assists"`
	CreepScore int     `json:"creepScore"`
	Deaths     int     `json:"deaths"`
	Kills      int     `json:"kills"`
	WardScore  float64 `json:"wardScore"`
}

type Item struct {
	CanUse         bool   `json:"canUse"`
	Consumable     bool   `json:"consumable"`
	Count          int    `json:"count"`
	DisplayName    string `json:"displayName"`
	ItemID         int    `json:"itemID"`
	Price          int    `json:"price"`
	RawDescription string `json:"rawDescription"` // discarded in transform
	RawDisplayName string `json:"rawDisplayName"` // discarded in transform
	Slot           int    `json:"slot"`           // 0-6, 6 = trinket
}

type SummonerSpells struct {
	SummonerSpellOne SummonerSpell `json:"summonerSpellOne"`
	SummonerSpellTwo SummonerSpell `json:"summonerSpellTwo"`
}

type SummonerSpell struct {
	DisplayName    string `json:"displayName"`
	RawDescription string `json:"rawDescription"`
	RawDisplayName string `json:"rawDisplayName"`
}

type ActivePlayer struct {
	CurrentGold   float64       `json:"currentGold"`
	Level         int           `json:"level"`
	RiotID        string        `json:"riotId"`
	ChampionStats ChampionStats `json:"championStats"`
	Abilities     Abilities     `json:"abilities"`
	FullRunes     FullRunes     `json:"fullRunes"`
}

// FullRunes is the spectated player's whole rune page. GeneralRunes arrives as
// one flat list of six: the keystone, then the three minor runes of the
// primary tree, then the two from the secondary. The client does not label
// which is which, so the split is positional.
type FullRunes struct {
	GeneralRunes      []Rune     `json:"generalRunes"`
	Keystone          Rune       `json:"keystone"`
	PrimaryRuneTree   Rune       `json:"primaryRuneTree"`
	SecondaryRuneTree Rune       `json:"secondaryRuneTree"`
	StatRunes         []StatRune `json:"statRunes"`
}

// StatRune is a stat shard. Only the perk ID is meaningful; the name is a
// localisation key rather than anything renderable.
type StatRune struct {
	ID int `json:"id"`
}

type ChampionStats struct {
	AbilityPower  float64 `json:"abilityPower"`
	Armor         float64 `json:"armor"`
	AttackDamage  float64 `json:"attackDamage"`
	AttackSpeed   float64 `json:"attackSpeed"`
	CurrentHealth float64 `json:"currentHealth"`
	MaxHealth     float64 `json:"maxHealth"`
	MagicResist   float64 `json:"magicResist"`
	MoveSpeed     float64 `json:"moveSpeed"`
	ResourceValue float64 `json:"resourceValue"`
	ResourceMax   float64 `json:"resourceMax"`
	AbilityHaste  float64 `json:"abilityHaste"`
}

type Abilities struct {
	Q Ability `json:"Q"`
	W Ability `json:"W"`
	E Ability `json:"E"`
	R Ability `json:"R"`
}

type Ability struct {
	AbilityLevel int    `json:"abilityLevel"`
	DisplayName  string `json:"displayName"`
	ID           string `json:"id"`
}

type Events struct {
	Events []Event `json:"Events"`
}

// Event is one struct for all event types - fields are absent for events that
// don't use them. The set of EventNames is open; unknown names are passed
// through to the relay rather than dropped.
type Event struct {
	EventID      int      `json:"EventID"`
	EventName    string   `json:"EventName"`
	EventTime    float64  `json:"EventTime"`
	KillerName   string   `json:"KillerName,omitempty"`
	VictimName   string   `json:"VictimName,omitempty"`
	Assisters    []string `json:"Assisters,omitempty"`
	DragonType   string   `json:"DragonType,omitempty"` // Air, Earth, Fire, Water, Hextech, Chemtech, Elder
	Stolen       string   `json:"Stolen,omitempty"`     // "True" / "False" - string, not bool
	TurretKilled string   `json:"TurretKilled,omitempty"`
	InhibKilled  string   `json:"InhibKilled,omitempty"`
	// FirstBlood names its player in Recipient, not KillerName. Riot's own
	// sample event list omits FirstBlood entirely, so the spec never mentioned
	// this and the event reached the frontend with no killer at all - it read
	// "First blood - something" for every game.
	Recipient string `json:"Recipient,omitempty"`
	// Each inhibitor event keys the structure name to its OWN event name rather
	// than to a shared field, so respawn events carry nothing under InhibKilled.
	// Without these the frontend could not tell which inhibitor came back, and
	// the minimap left it destroyed for the rest of the game.
	InhibRespawned      string `json:"InhibRespawned,omitempty"`
	InhibRespawningSoon string `json:"InhibRespawningSoon,omitempty"`
	KillStreak   int      `json:"KillStreak,omitempty"`
	Acer         string   `json:"Acer,omitempty"`
	AcingTeam    string   `json:"AcingTeam,omitempty"`
	Result       string   `json:"Result,omitempty"` // GameEnd: "Win" / "Lose"
}
