package quest

// PATCH-DEPENDENT VALUES — RE-CHECK EVERY PRESEASON.
//
// Everything in this file describes game content that Riot re-tunes. If a
// quest stops firing, this is the first file to look at.
//
// How to verify against a real game:
//
//	lolticker-agent --dump raw.jsonl      # play until someone finishes the quest
//	# then read the item names and IDs straight out of the recording:
//	python -c "import json;[print(i['itemID'],i['displayName']) for l in open('raw.jsonl') for p in json.loads(l)['allPlayers'] for i in p['items']]" | sort -u

// Tier3Boots holds the itemIDs of the tier-3 boots that completing the
// mid-lane quest grants. Such an ID in a player's inventory is the signal.
//
// THIS SET IS EMPTY ON PURPOSE. The v2 spec says to verify these IDs against a
// real --dump before hardcoding them, and no recording available during
// development contained tier-3 boots — the only boots seen was tier-1 Boots
// (1001), six minutes into a game. Guessed IDs would either match the wrong
// item or silently never fire, which is worse than an obviously unfinished
// list.
//
// To finish this: run the command above on a game where someone upgrades their
// boots and add the IDs here. Nothing else needs to change. The agent logs a
// warning at startup while this is empty.
var Tier3Boots = map[int]bool{
	// 0000: true, // Name Of The Boots
}

// MaxLevelWithoutQuest is the normal level cap. The top-lane quest raises it,
// so anything above this is a completed quest.
const MaxLevelWithoutQuest = 18

// BaseTeleportKey is the unupgraded Teleport. Any other spell key containing
// "Teleport" is an upgraded variant, and therefore a completed top-lane quest.
//
// Matching on "contains Teleport but is not the base key" rather than on a
// hardcoded variant name is deliberate: the variant key has changed name
// across patches before, and this survives that.
const BaseTeleportKey = "SummonerTeleport"

// BotQuestItemCount is the inventory size that means the bot-lane quest is
// done: six items plus a trinket is seven, and the quest grants an eighth slot.
const BotQuestItemCount = 7
