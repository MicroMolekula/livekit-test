package main

import (
	"context"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

func main() {
	hostURL := "http://localhost:7880" // ex: https://project-123456.livekit.cloud
	apiKey := "devkey"
	apiSecret := "secret"

	roomName := "myroom"

	roomClient := lksdk.NewRoomServiceClient(hostURL, apiKey, apiSecret)

	// create a new room
	roomClient.CreateRoom(context.Background(), &livekit.CreateRoomRequest{
		Name: roomName,
	})
}
