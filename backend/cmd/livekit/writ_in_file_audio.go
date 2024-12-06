package main

import (
	"backend/cloudapi/output/github.com/yandex-cloud/go-genproto/yandex/cloud/ai/stt/v3"
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
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var chunkCh = make(chan []byte, 2)

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
	go getStream()
	room, err := lksdk.ConnectToRoom("ws://localhost:7880", lksdk.ConnectInfo{
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
	bytePacket, err := packet.Marshal()
	if err != nil {
		panic(err)
	}
	fmt.Println(bytePacket)
	chunkCh <- bytePacket
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

func getStream() {

	recognizeOptions := &stt.StreamingOptions{
		RecognitionModel: &stt.RecognitionModelOptions{
			AudioFormat: &stt.AudioFormatOptions{
				AudioFormat: &stt.AudioFormatOptions_ContainerAudio{
					ContainerAudio: &stt.ContainerAudio{
						ContainerAudioType: stt.ContainerAudio_OGG_OPUS,
					},
				},
			},
			TextNormalization: &stt.TextNormalizationOptions{
				TextNormalization: stt.TextNormalizationOptions_TEXT_NORMALIZATION_ENABLED,
				ProfanityFilter:   true,
				LiteratureText:    false,
			},
			LanguageRestriction: &stt.LanguageRestrictionOptions{
				RestrictionType: stt.LanguageRestrictionOptions_WHITELIST,
				LanguageCode:    []string{"ru-RU"},
			},
			AudioProcessingType: stt.RecognitionModelOptions_REAL_TIME,
		},
	}

	grpclog.SetLoggerV2(grpclog.NewLoggerV2(os.Stdout, os.Stderr, os.Stderr))
	grpcConn, err := grpc.NewClient(
		"stt.api.cloud.yandex.net",
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})),
		grpc.WithPerRPCCredentials(&tokenAuth{"Api-Key AQVN3YdhnRO9_90yFRj8f4Mr_WTXcXhjWaSITTE5"}),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16*1024*1024)),
	)
	if err != nil {
		panic(err)
	}
	//defer grpcConn.Close()
	client := stt.NewRecognizerClient(grpcConn)
	ctx := context.Background()
	stream, err := client.RecognizeStreaming(ctx)
	if err != nil {
		panic(err)
	}
	err = stream.Send(&stt.StreamingRequest{
		Event: &stt.StreamingRequest_SessionOptions{
			SessionOptions: recognizeOptions,
		},
	})
	if err != nil {
		panic(err)
	}
	go func() {
		for chunk := range chunkCh {
			err = stream.Send(&stt.StreamingRequest{
				Event: &stt.StreamingRequest_Chunk{
					Chunk: &stt.AudioChunk{Data: chunk},
				},
			})
			if err != nil {
				panic(err)
			}
		}
	}()

	go func() {
		res, err := stream.Recv()
		if err != nil {
			panic(err)
		}
		fmt.Println(res)
	}()
}
