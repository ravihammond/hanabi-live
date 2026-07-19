package main

import (
	"context"
	"testing"
)

func TestResearchGuestSkipsPersistentAccountHydration(t *testing.T) {
	data := websocketConnectDataForIdentity(
		context.Background(),
		nil,
		-100,
		"HSM Debug Spectator",
		true,
	)

	if data == nil || data.Friends == nil || data.ReverseFriends == nil {
		t.Fatal("research guest did not receive initialized guest connection data")
	}
}
