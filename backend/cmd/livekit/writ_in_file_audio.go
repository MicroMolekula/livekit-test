package main

import (
	protocol "backend/pkg/salute_speech"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/livekit/server-sdk-go/v2/pkg/samplebuilder"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/protobuf/types/known/durationpb"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var chunkCh = make(chan []byte, 2)

func main() {
	apiKey := "devkey"
	apiSecret := "secret"
	roomName := "myroom"
	identity := "botuser"

	roomCB := &lksdk.RoomCallback{
		ParticipantCallback: lksdk.ParticipantCallback{
			OnTrackSubscribed: onTrackSubscribed,
		},
	}

	room, err := lksdk.ConnectToRoom("ws://localhost:7880", lksdk.ConnectInfo{
		APIKey:              apiKey,
		APISecret:           apiSecret,
		RoomName:            roomName,
		ParticipantIdentity: identity,
	}, roomCB)
	if err != nil {
		panic(err)
	}
	go Recognition()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	<-sigChan
	room.Disconnect()
}

type TrackWriter struct {
	sb     *samplebuilder.SampleBuilder
	writer media.Writer
	track  *webrtc.TrackRemote
}

func onTrackSubscribed(track *webrtc.TrackRemote, publication *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	packet, _, err := track.ReadRTP()
	if err != nil {
		panic(err)
	}
	chunkPkt, err := packet.Marshal()
	if err != nil {
		panic(err)
	}
	chunkCh <- chunkPkt
}

const (
	maxVideoLate = 1000 // nearly 2s for fhd video
	maxAudioLate = 100  // 4s for audio
)

func NewTrackWriter(track *webrtc.TrackRemote, fileName string) (*TrackWriter, error) {
	var (
		sb     *samplebuilder.SampleBuilder
		writer media.Writer
		err    error
	)
	switch {
	case strings.EqualFold(track.Codec().MimeType, "audio/opus"):
		sb = samplebuilder.New(maxAudioLate, &codecs.OpusPacket{}, track.Codec().ClockRate)
		writer, err = oggwriter.New(fileName+".ogg", 48000, track.Codec().Channels)
	default:
		return nil, errors.New("unsupported codec type")
	}
	if err != nil {
		return nil, err
	}
	t := &TrackWriter{
		sb:     sb,
		writer: writer,
		track:  track,
	}
	go t.start()
	return t, nil
}

func (t *TrackWriter) start() {
	defer t.writer.Close()
	for {
		pkt, _, err := t.track.ReadRTP()
		if err != nil {
			break
		}
		t.sb.Push(pkt)

		for _, p := range t.sb.PopPackets() {
			t.writer.WriteRTP(p)
		}
	}
}

type tokenAuth struct {
	Token string
}

func (t *tokenAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": t.Token,
	}, nil
}

func (t *tokenAuth) RequireTransportSecurity() bool {
	return false
}

func Recognition() {
	grpclog.SetLoggerV2(grpclog.NewLoggerV2(os.Stdout, os.Stderr, os.Stderr))
	grpcConn, err := grpc.NewClient(
		"smartspeech.sber.ru",
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})),
		grpc.WithPerRPCCredentials(&tokenAuth{"Bearer ?"}),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16*1024*1024)),
	)
	if err != nil {
		panic(err)
	}

	recognitionOptions := &protocol.RecognitionOptions{
		AudioEncoding:    protocol.RecognitionOptions_OPUS,
		Language:         "ru-RU",
		Model:            "general",
		HypothesesCount:  1,
		NoSpeechTimeout:  &durationpb.Duration{Seconds: 7},
		MaxSpeechTimeout: &durationpb.Duration{Seconds: 20},
	}
	client := protocol.NewSmartSpeechClient(grpcConn)
	ctx := context.Background()
	stream, err := client.Recognize(ctx)
	if err != nil {
		panic(err)
	}
	go func(chunkCh chan []byte) {
		for ch := range chunkCh {
			errSend := stream.Send(
				&protocol.RecognitionRequest{
					Request: &protocol.RecognitionRequest_Options{
						Options: recognitionOptions,
					},
				},
			)
			if errSend != nil {
				panic(errSend)
			}
			errSend = stream.Send(&protocol.RecognitionRequest{
				Request: &protocol.RecognitionRequest_AudioChunk{
					AudioChunk: ch,
				},
			})
		}
		stream.CloseSend()
	}(chunkCh)
	go func() {
		for {
			resp, errRecv := stream.Recv()
			if errRecv != nil {
				panic(errRecv)
			}
			if !resp.Eou {
				break
			}
			fmt.Println(resp)
		}
	}()
}
