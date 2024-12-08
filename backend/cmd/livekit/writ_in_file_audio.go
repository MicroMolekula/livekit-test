package main

import (
	"backend/cloudapi/output/github.com/yandex-cloud/go-genproto/yandex/cloud/ai/stt/v3"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	hropus "github.com/hraban/opus"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/livekit/server-sdk-go/v2/pkg/samplebuilder"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	sampleRate = 48000 // Частота дискретизации для SpeechKit
	channels   = 1     // Моно
	frameSize  = 960   // Количество сэмплов на кадр (20ms при 48kHz)
)

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

	go recognize()

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
	acamulatedData := make([]byte, 1)
	for {
		opusData := make([]byte, 960*2) // Буфер для Opus-фреймов
		n, _, err := track.Read(opusData)
		if err != nil {
			log.Println("Ошибка чтения аудиоданных:", err)
			return
		}
		pcmBytes := opusToPCM(opusData[:n])
		acamulatedData = append(acamulatedData, pcmBytes...)
		if len(acamulatedData) >= 4096 {
			fmt.Println(acamulatedData)
			chunkCh <- acamulatedData
			acamulatedData = make([]byte, 1)
		}
	}
}

func opusToPCM(in []byte) []byte {
	decoder, err := hropus.NewDecoder(48000, 1)
	if err != nil {
		fmt.Println("Ошибка при создании декодера", err)
	}
	pcmBytes := make([]int16, 960*2)
	n, err := decoder.Decode(in, pcmBytes)
	if err != nil {
		fmt.Println("Ошибка при декодировании", err)
	}
	return pcmToBytes(pcmBytes[:n])
}

func pcmToBytes(pcmData []int16) []byte {
	// Создаем новый буфер для байтового массива
	buf := new(bytes.Buffer)

	// Преобразуем каждый int16 в 2 байта и записываем в буфер
	for _, sample := range pcmData {
		// Записываем каждый int16 как два байта в Little Endian
		err := binary.Write(buf, binary.LittleEndian, sample)
		if err != nil {
			fmt.Println("Ошибка при записи в буфер:", err)
		}
	}

	// Возвращаем байтовый массив
	return buf.Bytes()
}

func recognize() {
	recognizeOptions := &stt.StreamingOptions{
		RecognitionModel: &stt.RecognitionModelOptions{
			AudioFormat: &stt.AudioFormatOptions{
				AudioFormat: &stt.AudioFormatOptions_RawAudio{
					RawAudio: &stt.RawAudio{
						AudioEncoding:     stt.RawAudio_LINEAR16_PCM,
						SampleRateHertz:   sampleRate,
						AudioChannelCount: 1,
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
		"stt.api.cloud.yandex.net:443",
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})),
		grpc.WithPerRPCCredentials(&tokenAuth{"Api-Key AQVN3YdhnRO9_90yFRj8f4Mr_WTXcXhjWaSITTE5"}),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(4*1024*1025)),
	)
	defer grpcConn.Close()
	if err != nil {
		panic(err)
	}
	//defer grpcConn.Close()
	client := stt.NewRecognizerClient(grpcConn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
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
		for audioData := range chunkCh {
			err := stream.Send(&stt.StreamingRequest{
				Event: &stt.StreamingRequest_Chunk{
					Chunk: &stt.AudioChunk{Data: audioData},
				},
			})
			if err != nil {
				fmt.Println("Ошибка отправки аудио в gRPC:", err)
				break
			}
		}
		stream.CloseSend()
	}()

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			fmt.Println("Удален")
			break
		}
		if err != nil {
			log.Fatalf("Ошибка получения ответа: %v", err)
		}

		fmt.Println("Результат распознавания:", resp.GetSpeakerAnalysis())
	}
}
