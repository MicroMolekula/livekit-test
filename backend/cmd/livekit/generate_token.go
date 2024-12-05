package main

import (
	"fmt"
	"github.com/livekit/protocol/auth"
	"time"
)

func main() {
	apiKey := "devkey"
	apiSecret := "secret"

	roomName := "myroom"
	identity := "participantIdentity1"
	fmt.Println(getJoinToken(apiKey, apiSecret, roomName, identity))
}

func getJoinToken(apiKey, apiSecret, room, identity string) (string, error) {
	at := auth.NewAccessToken(apiKey, apiSecret)
	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     room,
	}
	at.AddGrant(grant).
		SetIdentity(identity).
		SetValidFor(time.Hour)

	return at.ToJWT()
}
