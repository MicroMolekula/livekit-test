package recognize

import (
	trnl "backend/internal/services/translate"
	"bytes"
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/livekit/server-sdk-go/v2/pkg/samplebuilder"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var resultRecognizeEn = make(chan string, 1)

func outputResult() {
	router := gin.Default()
	router.GET("/en", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for r := range resultRecognizeEn {
			conn.WriteMessage(websocket.TextMessage, []byte(r))
		}
	})
	router.Run("localhost:8088")
}

func Recognize(url, apiKey, apiSecret, roomName, identity string) {
	go outputResult()
	roomCB := &lksdk.RoomCallback{
		ParticipantCallback: lksdk.ParticipantCallback{
			OnTrackSubscribed: onTrackSubscribed,
		},
	}

	room, err := lksdk.ConnectToRoom(url, lksdk.ConnectInfo{
		APIKey:              apiKey,
		APISecret:           apiSecret,
		RoomName:            roomName,
		ParticipantIdentity: identity,
	}, roomCB)
	if err != nil {
		panic(err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	<-sigChan
	room.Disconnect()
}

func onTrackSubscribed(track *webrtc.TrackRemote, publication *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	var channelIn = make(chan []byte, 1)
	var channelOut = make(chan *ResultRecognizer, 1)
	var textRec = make(chan string, 1)
	go steamTrackToOggOpus(track, channelIn)
	go SpeechKitRecognize(channelIn, channelOut)
	go uniqueResult(channelOut, textRec)
	go translate(textRec)
}

func steamTrackToOggOpus(track *webrtc.TrackRemote, channelIn chan<- []byte) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in steamTrackToOggOpus", r)
			close(channelIn)
		}
	}()
	sb := samplebuilder.New(200, &codecs.OpusPacket{}, 48000)
	oggBuffer := new(bytes.Buffer)
	writer, err := oggwriter.NewWith(oggBuffer, track.Codec().ClockRate, track.Codec().Channels)
	if err != nil {
		fmt.Println("Ошибка создания врайтера", err)
	}
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			fmt.Println("Ошибка чтения данных из трека", err)
		}
		sb.Push(pkt)
		for _, p := range sb.PopPackets() {
			if err := writer.WriteRTP(p); err != nil {
				fmt.Println("Ошибка записи RTP в OGG:", err)
			}
		}
		if len(oggBuffer.Bytes()) >= 2046 {
			channelIn <- oggBuffer.Bytes()
			oggBuffer.Reset()
		}
	}
}

func uniqueResult(channelIn chan *ResultRecognizer, channelOut chan string) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("Recover uniqueResult", err)
			close(channelIn)
		}
	}()
	var currentResult = ""
	for r := range channelIn {
		//resultRecognizeEn <- r.Text
		if currentResult != r.Text {
			currentResult = r.Text
			channelOut <- r.Text
		}
	}
}

func translate(channel chan string) {
	traslateService, err := trnl.NewServ()
	if err != nil {
		fmt.Println("Ошибка создания сервиса перевода", err)
		return
	}
	defer traslateService.CloseConn()
	ctx := context.Background()
	for s := range channel {
		result, err := traslateService.TranslateText(ctx, s)
		if err != nil {
			fmt.Println("Ошибка перевода", err)
		}
		resultRecognizeEn <- result
	}
}
