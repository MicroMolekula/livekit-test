package main

import (
	"backend/cloudapi/output/github.com/yandex-cloud/go-genproto/yandex/cloud/ai/stt/v3"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"gopkg.in/hraban/opus.v2"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	sampleRate = 48000 // Частота дискретизации для SpeechKiн
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

var chunkCh = make(chan []byte)

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

	//go recognize()
	go inFile()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	<-sigChan
	room.Disconnect()
}

func onTrackSubscribed(track *webrtc.TrackRemote, publication *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	acamulatedData := make([]byte, 1)
	for {
		pkt, _, err := track.ReadRTP()
		fmt.Println(pkt)
		if err != nil {
			fmt.Println("Ошибка чтения данных из трека", err)
		}
		pcm, err := getAudioRawBuffer(pkt.Payload)
		if err != nil {
			fmt.Println("Ошибка декодирования", err)
		}
		acamulatedData = append(acamulatedData, pcm...)
		if len(acamulatedData) >= 4096 {
			if err != nil {
				fmt.Println("Ошибка декодирования", err)
			}
			chunkCh <- pcm
			acamulatedData = make([]byte, 1)
		}
	}
}

func getAudioRawBuffer(fileBody []byte) ([]byte, error) {
	channels := 2
	s, err := opus.NewStream(bytes.NewReader(fileBody))
	if err != nil {
		return nil, err
	}
	defer s.Close()
	audioRawBuffer := new(bytes.Buffer)
	pcmbuf := make([]int16, 16384)
	for {
		n, err := s.Read(pcmbuf)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		pcm := pcmbuf[:n*channels]
		err = binary.Write(audioRawBuffer, binary.LittleEndian, pcm)
		if err != nil {
			return nil, err
		}
	}
	return audioRawBuffer.Bytes(), nil
}

func inFile() {
	// Открытие или создание файла для записи
	file, err := os.Create("output.pcm")
	if err != nil {
		fmt.Println("Ошибка при создании файла:", err)
		return
	}
	defer file.Close()

	for ch := range chunkCh {
		_, err := file.Write(ch)
		if err != nil {
			fmt.Println("Ошибка записи в файл", err)
		}
	}

	fmt.Println("Данные записаны в файл успешно!")
}

func recognize() {
	recognizeOptions := &stt.StreamingOptions{
		RecognitionModel: &stt.RecognitionModelOptions{
			AudioFormat: &stt.AudioFormatOptions{
				AudioFormat: &stt.AudioFormatOptions_RawAudio{
					RawAudio: &stt.RawAudio{
						AudioEncoding:     stt.RawAudio_LINEAR16_PCM,
						SampleRateHertz:   44100,
						AudioChannelCount: 2,
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
		grpc.WithPerRPCCredentials(&tokenAuth{"Api-Key "}),
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

		if resp.GetPartial() != nil {
			fmt.Println("Результат распознавания:", resp.GetPartial().Alternatives)
		}
	}
}
