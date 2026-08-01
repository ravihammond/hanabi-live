package main

// CheckScrub removes some information from the action to prevent players having more knowledge
// than they should have, if necessary (e.g. when a card is drawn to a player's hand)
func CheckScrub(t *Table, action interface{}, userID int) interface{} {
	return checkScrubForPlayer(t, action, getEquivalentPlayer(t, userID))
}

// CheckScrubForPlayerIndex applies the ordinary Hanabi.live player-visibility rules for an explicit
// physical seat. Unified projections use this seam instead of manufacturing an omniscient client.
func CheckScrubForPlayerIndex(t *Table, action interface{}, playerIndex int) interface{} {
	if playerIndex < 0 || playerIndex >= len(t.Game.Players) {
		return action
	}
	return checkScrubForPlayer(t, action, t.Game.Players[playerIndex])
}

func checkScrubForPlayer(t *Table, action interface{}, player *GamePlayer) interface{} {
	cardIdentityAction, ok := action.(ActionCardIdentity)
	if ok && cardIdentityAction.Type == "cardIdentity" {
		cardIdentityAction.scrubForPlayer(player)
		return cardIdentityAction
	}

	discardAction, ok := action.(ActionDiscard)
	if ok && discardAction.Type == "discard" {
		discardAction.scrubForPlayer(t, player)
		return discardAction
	}

	drawAction, ok := action.(ActionDraw)
	if ok && drawAction.Type == "draw" {
		drawAction.scrubForPlayer(t, player)
		return drawAction
	}

	playAction, ok := action.(ActionPlay)
	if ok && playAction.Type == "play" {
		playAction.scrubForPlayer(t, player)
		return playAction
	}

	return action
}

// Scrub removes some information from a draw so that we do not reveal the identity of drawn
// cards to the players drawing those cards
func (a *ActionDraw) Scrub(t *Table, userID int) {
	a.scrubForPlayer(t, getEquivalentPlayer(t, userID))
}

func (a *ActionDraw) scrubForPlayer(t *Table, p *GamePlayer) {
	// Local variables
	g := t.Game

	if p == nil {
		// Spectators get to see the identities of all drawn cards
		return
	}

	if a.PlayerIndex == p.Index || // They are drawing the card
		// They are playing a special character that should not be able to see the card
		characterHideCard(a, g, p) {

		a.Rank = -1
		a.SuitIndex = -1
	}
}

// Scrub removes some information from played cards so that we do not reveal the identity of played
// cards to anybody (in some specific variants)
func (a *ActionPlay) Scrub(t *Table, userID int) {
	a.scrubForPlayer(t, getEquivalentPlayer(t, userID))
}

func (a *ActionPlay) scrubForPlayer(t *Table, p *GamePlayer) {
	// Local variables
	variant := variants[t.Options.VariantName]

	if p == nil {
		// Spectators get to see the identities of played cards
		return
	}

	if variant.IsThrowItInAHole() {
		a.Rank = -1
		a.SuitIndex = -1
	}
}

// Scrub removes some information from discarded cards so that we do not reveal the identity of
// discarded cards to anybody (in some specific variants)
func (a *ActionDiscard) Scrub(t *Table, userID int) {
	a.scrubForPlayer(t, getEquivalentPlayer(t, userID))
}

func (a *ActionDiscard) scrubForPlayer(t *Table, p *GamePlayer) {
	// Local variables
	variant := variants[t.Options.VariantName]

	if p == nil {
		// Spectators get to see the identities of discarded cards
		return
	}

	if variant.IsThrowItInAHole() && a.Failed {
		// For the purposes of hiding information, failed discards are equivalent to plays
		a.Rank = -1
		a.SuitIndex = -1
	}
}

// Scrub removes some information from a card identity action so that we do not reveal the identity
// of sliding cards to the players who are holding those cards
func (a *ActionCardIdentity) Scrub(t *Table, userID int) {
	a.scrubForPlayer(getEquivalentPlayer(t, userID))
}

func (a *ActionCardIdentity) scrubForPlayer(p *GamePlayer) {

	if p == nil {
		// Spectators get to see the identities of all cards
		return
	}

	if a.PlayerIndex == p.Index { // They are holding the card
		a.Rank = -1
		a.SuitIndex = -1
	}
}

func getEquivalentPlayer(t *Table, userID int) *GamePlayer {
	// Local variables
	g := t.Game
	playerIndex := t.GetPlayerIndexFromID(userID)
	spectatorIndex := t.GetSpectatorIndexFromID(userID)

	if playerIndex > -1 {
		// The action is going to be sent to one of the active players
		return g.Players[playerIndex]
	} else if spectatorIndex > -1 && t.Spectators[spectatorIndex].ShadowingPlayerIndex != -1 {
		// The action is going to be sent to a spectator that is shadowing one of the active players
		return g.Players[t.Spectators[spectatorIndex].ShadowingPlayerIndex]
	}

	// The action is going to be sent to a spectator that can see every hand
	return nil
}
